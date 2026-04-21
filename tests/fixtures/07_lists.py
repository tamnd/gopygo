xs = [1, 2, 3, 4, 5]
print(xs)
print(len(xs))
print(xs[0])
print(xs[-1])
total = 0
for v in xs:
    total = total + v
print(total)
xs[0] = 99
print(xs)
print(3 in xs)
print(42 in xs)
