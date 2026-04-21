def fib_rec(n):
    if n < 2:
        return n
    return fib_rec(n - 1) + fib_rec(n - 2)

def fib_iter(n):
    a = 0
    b = 1
    for _ in range(n):
        a, b = b, a + b
    return a

for i in range(10):
    print(i, fib_rec(i), fib_iter(i))
