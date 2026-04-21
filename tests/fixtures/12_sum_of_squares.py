def square(x: int) -> int:
    return x * x


def sum_of_squares(n: int) -> int:
    total = 0
    for i in range(1, n + 1):
        total = total + square(i)
    return total


print(sum_of_squares(10))
print(sum_of_squares(100))
