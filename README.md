# Watch Together

- A simple way to watch videos in real time with multiple connected clients using websockets
- Implements real time play/pause and scrubbing, ability to add/remove videos, and OAuth authentication

# Demo

[![Demo](./media/thumb.jpg)](https://vimeo.com/1043972987?share=copy#t=0)

# Developing

### Server

- These env vars must be set in `.env`
  - AWS_URL=\<url\>
  - AWS_ACCESS_KEY_ID=\<access_key_id\>
  - AWS_REGION=us-east-1
  - AWS_SECRET_ACCESS_KEY=\<secret_access_key\>
  - AWS_S3_BUCKET=bucket-name
  - DATABASE_URL=postgres://test:test@test/test
  - GOOGLE_CLIENT_ID=\<google_client_id\>
  - GOOGLE_CLIENT_SECRET=\<google_client_secret\>
  - BASE_URL=http://localhost:8080
  - CLIENT_URL=http://localhost:5173
- `cargo run`

### Client

- `npm install`
- `npm run dev`

# TODO

- Bug sometimes when seeking, sends infinite play/pause messages
- Reconnect logic?
- Chat system?
