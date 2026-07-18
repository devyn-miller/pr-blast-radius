"""Discount calculation engine."""
from decimal import Decimal
from shared.money import Money

class DiscountEngine:
    def __init__(self):
        self.rules = []

    def apply_discount(self, base_price: Money, customer_tier: str) -> Money:
        """Calculate discounted price based on customer tier.

        This is the function we'll modify in the test to trigger
        a blast radius analysis - changing its signature affects
        any caller (like OrderService.price).
        """
        discount_rate = {
            "gold": Decimal("0.20"),
            "silver": Decimal("0.10"),
            "bronze": Decimal("0.05"),
        }.get(customer_tier, Decimal("0.0"))

        discount_amount = base_price * (1 - discount_rate)
        return Money(discount_amount.amount, base_price.currency)
