"""Protected assessment tests for the greet exercise.

Learners cannot edit this file. It is the hidden assessment boundary.

Each requirement check goes through check_assertion, which emits one
SAMPLE_ASSERT record (test id, stable assertion code, expected and actual
values) before raising on mismatch. The backend matches those records
against its own expectations; the codes label requirements, the values
substantiate the behavior.
"""


import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from solution import greet


def check_assertion(test_id, code, actual, expected):
    if actual != expected:
        print("SAMPLE_ASSERT " + json.dumps({
            "test": test_id,
            "code": code,
            "expected": expected,
            "actual": actual,
        }, sort_keys=True))
        raise AssertionError("%r != %r" % (actual, expected))


class TestGreet(unittest.TestCase):
    def test_basic_greeting(self):
        check_assertion("test_basic_greeting", "BASIC_GREETING",
                        greet("Alice"), "Hello, Alice!")

    def test_strips_whitespace(self):
        check_assertion("test_strips_whitespace", "STRIP_WHITESPACE",
                        greet("  Bob  "), "Hello, Bob!")

    def test_empty_name_is_stranger(self):
        check_assertion("test_empty_name_is_stranger", "STRANGER_FALLBACK",
                        greet("   "), "Hello, stranger!")

    def test_empty_string_is_stranger(self):
        check_assertion("test_empty_string_is_stranger", "STRANGER_FALLBACK",
                        greet(""), "Hello, stranger!")


if __name__ == "__main__":
    unittest.main(verbosity=2)
