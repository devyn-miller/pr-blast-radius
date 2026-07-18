"""Inventory management service."""

class InventoryService:
    def __init__(self):
        self.stock = {}

    def reserve(self, product_id: str, quantity: int) -> bool:
        """Reserve inventory for an order. Independent of pricing."""
        current = self.stock.get(product_id, 0)
        if current >= quantity:
            self.stock[product_id] = current - quantity
            return True
        return False

    def release(self, product_id: str, quantity: int):
        """Release reserved inventory back to stock."""
        self.stock[product_id] = self.stock.get(product_id, 0) + quantity
