# Watch Together

- A simple way to watch videos in real time with multiple connected clients using websocket
- Implements real time play/pause and scrubbing and the ability to add/remove videos

# Demo

[![Demo](./media/thumb.jpg)](https://vimeo.com/1043507662?share=copy)

# Developing

### Server

- These env vars must be set before running the command below
    - export AWS_URL=
        - For accessing S3 bucket
    - export AWS_ACCESS_KEY_ID=
        - self-explanatory
    - export AWS_REGION=
        - Your S3 region
    - export AWS_SECRET_ACCESS_KEY=
        - self-explanatory
    - export AWS_S3_BUCKET=
        - For accessing bucket that contains video files
    - export DB_URL=
        - Database URL
    - export CLIENT_ID=
        - Google OAuth client ID
    - export CLIENT_SECRET=
        - Google Oauth client secret
- `air` will run the server in watch mode, or simply run the server using `go run main.go`
