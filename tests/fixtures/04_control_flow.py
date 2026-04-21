x = 5
if x > 10:
    print("big")
elif x > 0:
    print("small positive")
else:
    print("non-positive")

i = 0
while i < 3:
    print("while", i)
    i = i + 1

for j in range(4):
    if j == 2:
        continue
    if j == 3:
        break
    print("for", j)

total = 0
for k in range(1, 6):
    total = total + k
print("total", total)
