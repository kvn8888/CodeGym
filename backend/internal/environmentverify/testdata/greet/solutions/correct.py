"""Reviewed-candidate correct solution for the greet exercise."""


def greet(name):
    cleaned = name.strip()
    if not cleaned:
        return "Hello, stranger!"
    return "Hello, " + cleaned + "!"
