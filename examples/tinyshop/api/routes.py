"""HTTP routes for TinyShop (Flask-style).

Four handlers depend on pricing through OrderService.price — POST /orders,
POST /quote, POST /cart and POST /checkout — so all four sit in the blast
radius of a DiscountEngine.apply_discount change, and price itself becomes
the graph's fan-in hotspot. The GET /inventory/<sku> handler only touches
InventoryService and is the untouched branch: it must stay out of the blast
radius no matter what pricing does.
"""
from flask import Flask

from services.inventory_service import InventoryService
from services.order_service import OrderService
from services.pricing.discount_engine import DiscountEngine

app = Flask(__name__)

orders = OrderService(DiscountEngine())
inventory = InventoryService()


@app.post("/orders")
def create_order():
    total = orders.price("sku-123", 2, "gold")
    return {"total": str(total.amount), "currency": total.currency}


@app.post("/quote")
def quote_order():
    total = orders.price("sku-123", 1, "silver")
    return {"quote": str(total.amount), "currency": total.currency}


@app.post("/cart")
def price_cart():
    subtotal = orders.price("sku-456", 3, "bronze")
    return {"subtotal": str(subtotal.amount), "currency": subtotal.currency}


@app.post("/checkout")
def checkout():
    total = orders.price("sku-789", 1, "gold")
    return {"charged": str(total.amount), "currency": total.currency}


@app.get("/inventory/<sku>")
def reserve_stock(sku):
    reserved = inventory.reserve(sku, 1)
    return {"sku": sku, "reserved": reserved}
