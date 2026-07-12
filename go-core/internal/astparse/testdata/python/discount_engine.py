from shared.money import round_currency
from services.inventory_service import InventoryService
from flask import Flask

app = Flask(__name__)


class DiscountEngine:
    def __init__(self):
        self.max_discount = 0.5

    def apply_discount(self, amount, rate):
        capped = min(rate, self.max_discount)
        discounted = amount * (1 - capped)
        return round_currency(discounted)


class OrderService:
    def __init__(self, inventory: InventoryService):
        self.inventory = inventory
        engine = DiscountEngine()
        self.engine = engine

    def price(self, amount, rate):
        return self.engine.apply_discount(amount, rate)

    def reserve_stock(self, sku):
        return self.inventory.reserve(sku)


@app.post("/orders")
def create_order():
    svc = OrderService(InventoryService())
    return svc.price(100.0, 0.1)