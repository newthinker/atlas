package hestia

import "context"

// History 是闸门层对历史数据的全部需求。
//
// 定义在消费方而不是 Store 那边：闸门只要「前 n 期」这一个能力，把它收成
// 单方法接口，单测就能注入假历史，不必为了测一个纯函数去建真库。
//
// 一个方法支撑两道闸（stock_continuity 用 n=1，deposit_sum 的漂移用 n=6），
// 闸门自己算。不为每道闸各开一个方法——那会让接口随闸门数量膨胀。
type History interface {
	// Preceding 返回 period 之前最近 n 期的当前行，按 period 降序。
	//
	// 库里没有历史时返回空切片而不是错误——首期入库是正常路径。
	Preceding(ctx context.Context, period, periodType string, n int) ([]Observation, error)
}

// NoHistory 是恒空的 History，给还没有库的调用方用（例如只想跑无历史闸门的
// dry-run）。
//
// 它存在的意义是让 Validate 能拒绝 nil：nil 一律是接线错误，而「确实没有
// 历史」有一个显式的值来表达。两者混用会让忘记传 Store 这种 bug 悄悄退化成
// 「所有需要历史的闸门都 skipped」。
var NoHistory History = noHistory{}

type noHistory struct{}

func (noHistory) Preceding(context.Context, string, string, int) ([]Observation, error) {
	return nil, nil
}
