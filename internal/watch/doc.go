// Package watch implements the check logic for watches, the index.published
// event and the notification sinks (WATCH-01).
//
// Check does one pass: for every watch, the latest published value of the
// series is compared against the months already seen in the store. Each
// genuinely new month produces exactly one event — recorded first (the unique
// key makes duplicates impossible, decision 2) and then delivered to the sink.
// An event recorded but not delivered (a previous sink failure) is retried on
// every pass before new months, without duplicating notifications (decision
// 5): a sink failure returns an error but never deletes the recorded event.
//
// The Notification is both the payload delivered to the sink and the event
// printed by `watch check`. Without --with-impact it carries no loan data at
// all: index, month, value, series and the Banco de España attribution only
// (decision 3, P-03). With --with-impact it adds the estimated payment change,
// marked as an estimate and accompanied by the P-03 warning; the estimate
// itself is computed by the injected ImpactFor (internal/app), never here.
//
// The only sink in v1 is the webhook (webhook.go): one JSON POST to the URL
// configured by the user, carrying the attribution text and its update date
// (decision 4). Every sink failure surfaces as upstream_unavailable (code 5).
package watch
