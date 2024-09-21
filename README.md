# Watch Together

-   Allows a websocket connection (password protected) to stream videos to multiple clients with real time video controlling
-   Lists videos from S3 bucket and allows user to choose one and have others connect to the same video "room"

# Developing

### Server

-   These env vars must be set before running the command below
    -   export AWS_URL=
        -   For accessing S3 bucket
    -   export AWS_ACCESS_KEY_ID=
        -   self-explanatory
    -   export AWS_REGION=
        -   Your S3 region
    -   export AWS_SECRET_ACCESS_KEY=
        -   self-explanatory
    -   export AWS_S3_BUCKET=
        -   For accessing bucket that contains video files
    -   export PASSWORD=
        -   Password to get into the video library
    -   export COOKIE_VAL=
        -   Cookie value you want to be set which the app will look for upon each request
-   `go run main.go` will run the Go backend
-   This project also uses [air](https://github.com/air-verse/air) for hot reloading

### Client

-   Since this project uses TailwindCSS for styling, you will have to generate the `dist/output.css` file either once or in watch mode during development
    -   Simply run `npx tailwindcss -i ./client/input.css -o ./dist/output.css --watch`

# TODO

-   [ ] Add spinner when clicking on video
-   [ ] Protect API endpoints
-   [ ] Overall refactor some code paths
