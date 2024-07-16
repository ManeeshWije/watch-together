# Watch Together

-   Allows a websocket connection (password protected) to stream videos to multiple clients

# Developing

### Server

-   These env vars must be set before running the command below
    -   export AWS_URL=
    -   export AWS_ACCESS_KEY_ID=
    -   export AWS_REGION=
    -   export AWS_SECRET_ACCESS_KEY=
    -   export AWS_S3_BUCKET=
    -   export PASSWORD=
-   `cargo watch -x run` will run the Rust backend in watch mode

### Client

-   This project uses Bun to build/run the TypeScript files and TailwindCSS for styling
-   To run both in watch mode, simply run `bun dev`
    -   This command uses the `concurrently` dev dependency to run each in a separate process
