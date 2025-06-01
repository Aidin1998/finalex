from locust import HttpUser, task, between
import random

class SpikeUser(HttpUser):
    wait_time = between(0.001, 0.01)

    @task
    def place_order(self):
        self.client.post("/api/v1/order", json={
            "symbol": "BTC-USD",
            "side": random.choice(["buy", "sell"]),
            "price": random.uniform(10000, 100000),
            "quantity": random.uniform(0.01, 2.0),
            "type": "limit",
            "user_id": random.randint(1, 1000000),
        })
