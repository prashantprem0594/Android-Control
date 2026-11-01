package parentpush

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"
)

const (
	workerBatchSize  = 10
	workerPollPeriod = time.Second
)

type Worker struct {
	store  *Store
	sender Sender
	logger *slog.Logger
}

func NewWorker(store *Store, sender Sender, logger *slog.Logger) (*Worker, error) {
	if store == nil || sender == nil {
		return nil, ErrInvalid
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Worker{store: store, sender: sender, logger: logger}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || ctx == nil {
		return ErrInvalid
	}
	ticker := time.NewTicker(workerPollPeriod)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			w.logger.Warn("parent push batch failed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOnce(ctx context.Context) error {
	deliveries, err := w.store.Claim(ctx, workerBatchSize)
	if err != nil {
		return err
	}
	for i, delivery := range deliveries {
		if ctx.Err() != nil {
			for _, pending := range deliveries[i:] {
				w.finish(ctx, pending, false)
				clear(pending.RegistrationToken)
			}
			return nil
		}
		w.processDelivery(ctx, delivery)
	}
	return nil
}

func (w *Worker) processDelivery(ctx context.Context, delivery Delivery) {
	defer clear(delivery.RegistrationToken)

	remaining, eligible, err := w.store.ValidateLease(ctx, delivery)
	if err != nil {
		if ctx.Err() != nil {
			w.finish(ctx, delivery, false)
		}
		w.logger.Warn("parent push lease validation failed")
		return
	}
	if !eligible {
		w.finish(ctx, delivery, false)
		return
	}
	ttl := time.Duration(int64(remaining/time.Second)) * time.Second
	if ttl <= 0 {
		w.finish(ctx, delivery, false)
		return
	}
	sendContext, cancel := context.WithTimeout(ctx, remaining)
	result, sendErr := w.sender.Send(sendContext, delivery.RegistrationToken, ttl)
	cancel()
	if sendErr == nil && result == SendAccepted {
		w.finish(ctx, delivery, true)
		return
	}
	w.finish(ctx, delivery, false)
	if sendErr == nil && result == SendUnregistered {
		if _, err := w.store.RemoveInvalidRegistration(ctx, delivery.ParentID, delivery.FamilyID,
			delivery.RegistrationVersion); err != nil && !errors.Is(err, ErrUnauthorized) {
			w.logger.Warn("parent push registration cleanup failed")
		}
		return
	}
	w.logger.Warn("parent push send attempt failed")
}

func (w *Worker) finish(ctx context.Context, delivery Delivery, succeeded bool) {
	finishContext := ctx
	cancel := func() {}
	if ctx.Err() != nil {
		finishContext, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	}
	defer cancel()
	if err := w.store.Finish(finishContext, delivery.ID, delivery.LeaseID, succeeded); err != nil &&
		!errors.Is(err, ErrUnauthorized) && finishContext.Err() == nil {
		w.logger.Warn("parent push delivery completion failed")
	}
}
