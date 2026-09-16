// Package sheets 把 Hestia 的观测投影到 Google Sheets 的年度表录入区。
//
// 🔴 **本包不认识数据库。** 它不接收 *hestia.Store，也不接收 *sql.DB——调用方
// 读库、组装成纯值再交进来（约束 C2）。
//
// 这条不是风格偏好。internal/hestia 有两道写口守卫（ADR-0003），本包是它们
// 射程内的第一个子包；TASK-001 已让 AST 那条下钻子目录，堵的是「有人在这里
// 写了写口」。而本条堵的是另一半：**根本没有句柄可写**。两道缺一不可——
// 守卫能被新写法绕过，没有句柄绕不过去。
//
// ⚠️ hestia.Store.DB() 是导出的且返回 *sql.DB。**任何人不要把它传进本包。**
//
// 出网：本包用默认 transport（走 ProxyFromEnvironment）。Sheets 在境外，
// 与 hestia/fetch.go 对 PBOC 的直连策略相反——别照抄那边的空 Transport{}，
// 那会让本包直连境外 API 而失败（约束 C7 要求同进程内三种策略）。
package sheets
