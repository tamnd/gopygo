def add(a, b):
    return a + b

def square(x):
    return x * x

def sum_of_squares(n):
    total = 0
    for i in range(n):
        total = total + square(i)
    return total

print(add(2, 3))
print(square(7))
print(sum_of_squares(5))

def fact(n):
    if n <= 1:
        return 1
    return n * fact(n - 1)

print(fact(6))
