from services.pricing.discount_engine import DiscountEngine
from services.inventory_service import InventoryService

class OrderService:
    def __init__(self, inventory: InventoryService):
        self.inventory = inventory
        self.engine = DiscountEngine()

    def price(self, amount, rate):
        return self.engine.apply_discount(amount, rate)