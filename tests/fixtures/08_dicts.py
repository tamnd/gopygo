d = {"a": 1, "b": 2}
print(d["a"])
print(d["b"])
d["c"] = 3
print(d["c"])
print("a" in d)
print("z" in d)
for k in ("a", "b", "c"):
    print(k, d[k])
