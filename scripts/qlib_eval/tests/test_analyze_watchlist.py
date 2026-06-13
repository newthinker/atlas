"""Context Checkpoint: QA fix F2 done_criteria -> test mapping
is_index 不得把低位港股(HK 前缀)误判为指数；HK 指数 HSI/HSCEI 不带 HK 前缀。
boundary  "HK00001/HK00700 -> False(港股证券非指数)"        -> test_is_index_hk_securities_false
functional "HSI/HSCEI -> True(港股指数)"                     -> test_is_index_hk_indexes_true
functional "SH000300/CSI930713 -> True(A股/中证指数仍判定)"   -> test_is_index_ashare_indexes_true
boundary  "SH600519 等个股 -> False"                          -> test_is_index_ashare_securities_false
"""

import analyze_watchlist


def test_is_index_hk_securities_false():
    # 回归核心：c[2:5] == "000" 旧逻辑会把 HK00001 误判为指数
    assert analyze_watchlist.is_index("HK00001") is False
    assert analyze_watchlist.is_index("HK00700") is False


def test_is_index_hk_indexes_true():
    assert analyze_watchlist.is_index("HSI") is True
    assert analyze_watchlist.is_index("HSCEI") is True


def test_is_index_ashare_indexes_true():
    assert analyze_watchlist.is_index("SH000300") is True
    assert analyze_watchlist.is_index("CSI930713") is True


def test_is_index_ashare_securities_false():
    assert analyze_watchlist.is_index("SH600519") is False
    assert analyze_watchlist.is_index("SZ399001") is True  # 深市 399 段指数
