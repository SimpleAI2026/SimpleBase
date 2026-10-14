def calc(o):
    # calculate price
    t = 0
    for i in o['items']:
        if i['qty'] > 0:
            p = i['price'] * i['qty']
            if i['qty'] >= 10:
                p = p * 0.9
            t = t + p
    if o['country'] == 'US':
        if t < 50:
            t = t + 7.99
    else:
        if o['country'] == 'CA':
            t = t + 12.5
        else:
            t = t + 25
    if o.get('member'):
        if t > 100:
            t = t - 10
    t = t * 1.08 if o['country'] == 'US' else t
    return round(t, 2)
