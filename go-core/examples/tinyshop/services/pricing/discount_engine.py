from shared.money import round_currency

class DiscountEngine:
    def apply_discount(self, amount, rate):
        return round_currency(amount * (1 - rate))