"""Parse a Python file and emit a compact JSON AST on stdout."""
import ast
import json
import sys


def n(node):
    if node is None:
        return None
    if isinstance(node, list):
        return [n(x) for x in node]
    if not isinstance(node, ast.AST):
        return node
    d = {"_t": type(node).__name__}
    for f in node._fields:
        d[f] = n(getattr(node, f, None))
    if hasattr(node, "lineno"):
        d["lineno"] = node.lineno
        d["col"] = node.col_offset
    return d


def main():
    path = sys.argv[1]
    with open(path, "rb") as f:
        src = f.read()
    tree = ast.parse(src, filename=path)
    json.dump(n(tree), sys.stdout)


if __name__ == "__main__":
    main()
