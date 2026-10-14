import sys, importlib.util
s = importlib.util.spec_from_file_location("m", sys.argv[1]); m = importlib.util.module_from_spec(s); s.loader.exec_module(m)
f = m.get_user_data
assert m.__all__ == ["get_user_data"] or "get_user_data" in m.__all__
assert f(3) == {"id": 3, "name": "user3"}
assert f(3, True) == {"id": 3, "name": "user3", "email": "user3@example.com"}
assert f(3, False, True) == {"id": 3, "name": "user3", "orders": [30, 31]}
assert f(uid=2, flag1=True, flag2=True)["orders"] == [20, 21]
assert f(3, flag1=1) == {"id": 3, "name": "user3", "email": "user3@example.com"}  # 1 == True
assert f(3, flag1="y") == {"id": 3, "name": "user3"}  # truthy non-True ignored
print("PASS")
