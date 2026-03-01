.PHONY: test test-backend test-frontend

test: test-backend test-frontend

test-backend:
	cd backend && go test ./... -race -count=1

test-frontend:
	cd frontend && npm test
