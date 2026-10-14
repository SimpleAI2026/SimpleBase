# differential: impl vs original on non-int / odd uids and flag values (outputs and exception types)
import sys, importlib.util, pathlib
def load(p, n):
    s = importlib.util.spec_from_file_location(n, p); m = importlib.util.module_from_spec(s); s.loader.exec_module(m); return m
new = load(sys.argv[1], "new"); old = load(pathlib.Path(__file__).with_name("input.py"), "old")
def call(f, *a, **k):
    try: return ("ok", f(*a, **k))
    except Exception as e: return ("raise", type(e).__name__)
bad = []
for uid in [3, 3.7, -2, True, 0, 2**40, "3", None]:
    for args in [(), (True,), (False, True), (True, True), (1, 1.0)]:
        o, n = call(old.get_user_data, uid, *args), call(new.get_user_data, uid, *args)
        if o != n: bad.append((uid, args, o, n))
print("PASS" if not bad else f"FAIL {len(bad)} diffs e.g. {bad[:2]}"); sys.exit(bool(bad))
