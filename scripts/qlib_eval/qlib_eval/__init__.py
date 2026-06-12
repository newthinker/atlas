"""qlib_eval — atlas 信号事件研究评估的薄评估层。

硬约束：``import qlib`` 禁止出现在任何模块顶层，只允许在 ``QlibPriceSource``
方法体内惰性导入——这是 pytest 零 qlib 依赖的机制保证（见 README 口径说明）。
"""
