"""pytest 配置：显式声明包搜索路径。

pytest 通过 rootdir 向上查找 conftest.py 并在 collect 前导入它，因此这里把
``scripts/qlib_eval`` 注入 ``sys.path``，保证无论从仓库根（hook 门禁的方式：
``scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/``）还是
从包目录运行，``import qlib_eval`` 都能解析。

不在此处显式声明时，裸 ``pytest`` 会因 cwd 不在 sys.path 而 ModuleNotFoundError；
``python -m pytest`` 仅因 cwd 注入 sys.path 的副作用碰巧能跑，不可依赖。
"""

import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent))
