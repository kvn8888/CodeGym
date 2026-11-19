def sum_even(numbers: list[int]) -> int:
    return sum(filter(lambda number: number % 2 == 0, numbers))
