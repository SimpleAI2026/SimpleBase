"""usersdk — published on PyPI; get_user_data is imported by external customers."""
__all__ = ["get_user_data"]

def get_user_data(uid, flag1=False, flag2=False):
    d = {"id": uid, "name": "user%d" % uid}
    if flag1 == True:
        d["email"] = "user%d@example.com" % uid
    if flag2 == True:
        d["orders"] = [uid * 10, uid * 10 + 1]
    return d
