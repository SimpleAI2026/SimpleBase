import sys, importlib.util
s = importlib.util.spec_from_file_location("m", sys.argv[1]); m = importlib.util.module_from_spec(s); s.loader.exec_module(m)
cases = [None, {}, {"vip": True}, {"vip": 1}, {"vip": "yes"}, {"vip": True, "points": 1001}, {"vip": True, "points": 1000},
         {"points": 1001}, {"points": 5}, {"points": -3}, {"points": 0.5}, {"vip": 1.0, "points": 2000}, {"vip": False, "points": 1000}]
exp = [None, "普通", "银牌", "银牌", "普通", "金牌", "银牌", "银牌", "铜牌", "普通", "铜牌", "金牌", "铜牌"]
got = [m.get_level(c) for c in cases]
bad = [(c, g, e) for c, g, e in zip(cases, got, exp) if g != e]
print("PASS" if not bad else f"FAIL {bad}"); sys.exit(bool(bad))
