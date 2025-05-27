package stripe

import (
	"io/ioutil"
	"net/http"
	"os"

	"github.com/stripe/stripe-go/v75/webhook"
	"go.uber.org/zap"
)

func WebhookHandler(logger *zap.Logger, enqueueApplyDeposit func(eventID, userID, intentID string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := os.Getenv("STRIPE_WEBHOOK_SECRET")
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("read body", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sig := r.Header.Get("Stripe-Signature")
		event, err := webhook.ConstructEvent(payload, sig, secret)
		if err != nil {
			logger.Error("signature verification failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if event.Type == "payment_intent.succeeded" {
			userID := event.Data.Object["metadata"].(map[string]interface{})["user_id"].(string)
			intentID := event.Data.Object["id"].(string)
			if err := enqueueApplyDeposit(event.ID, userID, intentID); err != nil {
				logger.Error("enqueue applyDeposit", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}
