"""Shared money type for TinyShop."""
from decimal import Decimal

class Money:
    def __init__(self, amount: Decimal, currency: str = "USD"):
        self.amount = amount
        self.currency = currency

    def __repr__(self):
        return f"Money({self.amount}, {self.currency})"

    def __add__(self, other):
        if self.currency != other.currency:
            raise ValueError("Cannot add different currencies")
        return Money(self.amount + other.amount, self.currency)

    def __mul__(self, factor):
        return Money(self.amount * Decimal(str(factor)), self.currency)
