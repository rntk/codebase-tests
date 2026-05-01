import json
from pathlib import Path

import pytest

from calc import add


@pytest.mark.parametrize("case_name", ["positive", "negative"], ids=["positive", "negative"])
def test_add(case_name: str) -> None:
    root = Path(__file__).parent / "tests" / "golden" / "test_add" / "test_add"
    with (root / f"{case_name}.in.json").open() as f:
        payload = json.load(f)
    got = add(payload["a"], payload["b"])
    with (root / f"{case_name}.out.json").open() as f:
        expected = json.load(f)
    assert got == expected["want"]


class TestCalculator:
    def test_class_add(self) -> None:
        assert add(2, 3) == 5
