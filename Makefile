.PHONY: up down test-api test-web test-face

up:
	docker compose up --build -d

down:
	docker compose down

test-api:
	curl -s http://localhost:8080/healthz

test-web:
	curl -s -o /dev/null -w "%{http_code}" http://localhost:3000

test-face:
	docker compose exec face-service python -c "import urllib.request; print(urllib.request.urlopen('http://localhost:8090/healthz').read().decode())"
