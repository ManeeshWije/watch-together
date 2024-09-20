# Watch Together

-   Allows a websocket connection (password protected) to stream videos to multiple clients with real time video controlling
-   Lists videos from S3 bucket and allows user to choose one and have others connect to the same video "room"

# Developing

### Server

-   These env vars must be set before running the command below
    -   export AWS_URL=
    -   export AWS_ACCESS_KEY_ID=
    -   export AWS_REGION=
    -   export AWS_SECRET_ACCESS_KEY=
    -   export AWS_S3_BUCKET=
    -   export PASSWORD=
-   `go run main.go` will run the Go backend
-   This project also uses [air](https://github.com/air-verse/air) for hot reloading

### Client

-   Since this project uses TailwindCSS for styling, you will have to generate the `dist/output.css` file either once or in watch mode during development
    -   Simply run `npx tailwindcss -i ./client/input.css -o ./dist/output.css --watch`
