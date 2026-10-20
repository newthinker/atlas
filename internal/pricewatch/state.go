package pricewatch

import "time"

// RuleState 是「资产 × 规则」的提醒状态（spec §5）。
type RuleState struct {
	Tier       int       // 已提醒过的最深一档
	SentInTier int       // 这一档已发次数
	LastSentAt time.Time // 上次发送时间；规则不满足时不清零
}

// SendKind 表示本轮是否需要发送以及发送类型。
type SendKind int

const (
	SendNone   SendKind = iota
	SendNew             // 新触发，或跌进更深一档
	SendRepeat          // 仍在已提醒过的档内，重复提醒
)

// Decide 按 spec §5 的四种情况依序判断，返回新状态与发送类型。
// tier 为本轮档位（规则不满足时为 0）。纯函数，不修改 prev。
func Decide(prev RuleState, tier int, now time.Time, p NotifyPolicy) (RuleState, SendKind) {
	// 从未发过（零值）也视为已满间隔
	intervalElapsed := now.Sub(prev.LastSentAt) >= p.RepeatInterval

	switch {
	case tier == 0: // 1. 规则不满足：清零，保留 LastSentAt
		return RuleState{LastSentAt: prev.LastSentAt}, SendNone
	case prev.Tier == 0: // 2. 从未满足进入满足：第 1 档带防抖
		if tier >= 2 || intervalElapsed {
			return RuleState{Tier: tier, SentInTier: 1, LastSentAt: now}, SendNew
		}
		return prev, SendNone
	case tier > prev.Tier: // 3. 跌进更深一档
		return RuleState{Tier: tier, SentInTier: 1, LastSentAt: now}, SendNew
	default: // 4. 仍在已提醒过的档内
		if prev.SentInTier < p.MaxPerTier && intervalElapsed {
			return RuleState{Tier: prev.Tier, SentInTier: prev.SentInTier + 1, LastSentAt: now}, SendRepeat
		}
		return prev, SendNone
	}
}
