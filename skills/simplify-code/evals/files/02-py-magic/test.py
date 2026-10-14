import sys, importlib.util, itertools
spec = importlib.util.spec_from_file_location("m", sys.argv[1]); m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
fn = getattr(m, 'calc')
GOLD = {}
cases = []
for country, member, qtys in itertools.product(['US', 'CA', 'DE'], [None, False, True], [[0], [1], [3, 0], [10], [9, 12], [-2, 5], [40]]):
    o = {'items': [{'price': 4.99, 'qty': q} for q in qtys], 'country': country}
    if member is not None: o['member'] = member
    cases.append(o)
import json
exp = json.load(open(sys.argv[2])) if len(sys.argv) > 2 else None
got = [fn(c) for c in cases]
if exp is None: json.dump(got, open('golden.json', 'w')); print('golden written'); sys.exit()
bad = [(c, g, e) for c, g, e in zip(cases, got, exp) if g != e]
print('PASS' if not bad else f'FAIL {len(bad)} mismatches e.g. {bad[0]}'); sys.exit(1 if bad else 0)
