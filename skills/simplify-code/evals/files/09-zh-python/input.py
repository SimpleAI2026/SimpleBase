def get_level(user):
    if user is not None:
        if user.get("vip") == True:
            if user.get("points", 0) > 1000:
                return "金牌"
            else:
                return "银牌"
        else:
            if user.get("points", 0) > 1000:
                return "银牌"
            else:
                if user.get("points", 0) > 0:
                    return "铜牌"
                else:
                    return "普通"
    else:
        return None
