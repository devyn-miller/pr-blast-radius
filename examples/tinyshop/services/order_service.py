"""Order processing service - orchestrates pricing and inventory."""
from decimal import Decimal
from shared.money import Money
from services.pricing.discount_engine import DiscountEngine
from services.inventory_service import InventoryService

class OrderService:
    def __init__(self, pricing: DiscountEngine):
        self.pricing = pricing
        self.inventory = InventoryService()

    def price(self, product_id: str, quantity: int, customer_tier: str) -> Money:
        """Calculate final order price.

        This method CALLS discount_engine.apply_discount, so it's in
        the blast radius when apply_discount's signature changes.
        """
        base = Money(Decimal("100.00"))
        return self.pricing.apply_discount(base, customer_tier)

    def reserve_inventory(self, product_id: str, quantity: int) -> bool:
        """Reserve inventory.

        This method calls inventory_service.reserve, which is INDEPENDENT
        of the discount engine - so it should NOT appear in the blast
        radius when apply_discount changes.
        """
        return self.inventory.reserve(product_id, quantity)
