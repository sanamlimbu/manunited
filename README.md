## manunited

This terminal app gives information about Manchester United fixtures and match results.

## Build

1.  Rapid API and API-Football
    Create Rapid API account and from it's marketplace subscribe to API-Football free plan.
2.  Environment variables
    Create .env file at root project folder with environment variables below. You can get this from Rapid API dashboard.
    ```
    RAPIDAPI_HOST=api-football-v1.p.rapidapi.com
    RAPIDAPI_KEY=*******************************
    ```
3.  Build binary
    Running command below will create `manunited` binary inside bin folder.
    ```
    go build -o bin/manunited main.go
    ```
