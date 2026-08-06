package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// FileStore 是跨进程的 JSON 配额账本。
//
// 存在的理由：atlas 的 `prism refresh` 形态是 launchd 拉起的**短命进程**
// （设计 §1.5），内存计数每次启动归零，配额根本不会生效。
//
// 账本形如：
//
//	{"tushare.daily_basic": {"window_start": "2026-08-06T00:00:00+08:00", "count": 3}}
//
// 「读 → 判窗口 → 计数 → 原子写」全程持 flock 排他锁，故多进程并发安全。
// 用独立的 <path>.lock 加锁而非账本文件本身：账本靠 rename 原子替换，
// 对被替换掉的 inode 加锁没有意义。
//
// 窗口判定与计数复用 take() / windowStart()，与 MemStore 是**同一套纯逻辑**——
// 两个后端必须语义一致，各写一遍就多一次漂移的机会。
//
// 平台：flock 是 unix 系统调用。本仓库只在 darwin/linux 运行，故不加
// build tag；将来若需 Windows 支持再拆 _unix.go / _other.go。
type FileStore struct {
	path string
	mu   sync.Mutex // 同进程内串行；跨进程由 flock 负责
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

// Take 实现 QuotaStore。任何 I/O 或解析失败都返回 (true, err)：调用方据此
// fail-open 放行并告警——账本损坏绝不能阻断降级链（设计 §4.4）。
//
// 账本损坏时不只是报错：read 返回空账本，本次计数据此重建并原子写回，
// **账本就此自愈**。若只报错而不重写，配额会永久静默失效（反审 A10）。
func (f *FileStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	unlock, err := f.lock()
	if err != nil {
		return true, err
	}
	defer unlock()

	// 解析失败时 read 返回空账本 + err：账本就此自愈重建，
	// 同时把 err 带出去让 Gate 告警。
	ledgers, readErr := f.read()
	ok, entry := take(ledgers[topic], q, now)
	if !ok {
		// 被拦下的请求没发出去，不计数也就无需写盘。
		return false, readErr
	}
	ledgers[topic] = entry
	if err := f.write(ledgers); err != nil {
		return true, err // 已放行，但账本没记上——必须告警
	}
	return true, readErr
}

// lock 建目录、开锁文件并取排他 flock，返回释放函数。
// 释放函数负责解锁**并关闭 fd**：长驻进程反复 Take 不能泄漏 fd。
func (f *FileStore) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return nil, fmt.Errorf("policy: quota dir: %w", err)
	}
	lf, err := os.OpenFile(f.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("policy: quota lock file: %w", err)
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, fmt.Errorf("policy: quota flock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
		_ = lf.Close()
	}, nil
}

// read 返回账本。文件不存在是正常的首次运行，不算错误；内容损坏则返回
// 空账本 + 错误（fail-open + 自愈重建）。
func (f *FileStore) read() (map[string]ledgerEntry, error) {
	raw, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return make(map[string]ledgerEntry), nil
	}
	if err != nil {
		return make(map[string]ledgerEntry), fmt.Errorf("policy: read quota ledger: %w", err)
	}
	var l map[string]ledgerEntry
	if err := json.Unmarshal(raw, &l); err != nil || l == nil {
		return make(map[string]ledgerEntry), fmt.Errorf("policy: quota ledger corrupted at %s: %w", f.path, err)
	}
	return l, nil
}

// write 以 temp + rename 原子替换账本，避免崩溃留下半截文件。
// 任何一步失败都清理临时文件，不在目录里留垃圾。
func (f *FileStore) write(l map[string]ledgerEntry) error {
	raw, err := json.Marshal(l)
	if err != nil {
		return fmt.Errorf("policy: encode quota ledger: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".collector-quota-*.json")
	if err != nil {
		return fmt.Errorf("policy: temp quota ledger: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("policy: write quota ledger: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("policy: close quota ledger: %w", err)
	}
	if err := os.Rename(tmpName, f.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("policy: rename quota ledger: %w", err)
	}
	return nil
}
