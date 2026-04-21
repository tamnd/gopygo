p = (1, 2)
print(p)
a, b = p
print(a, b)
a, b = b, a
print(a, b)
x, y, z = 10, 20, 30
print(x, y, z)

def minmax(xs):
    lo = xs[0]
    hi = xs[0]
    for v in xs:
        if v < lo:
            lo = v
        if v > hi:
            hi = v
    return lo, hi

lo, hi = minmax((3, 1, 4, 1, 5, 9, 2, 6))
print(lo, hi)
