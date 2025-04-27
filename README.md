# Watch Together

- A simple way to watch videos in real time with multiple connected clients using websockets
- Implements real time play/pause and scrubbing, ability to add/remove videos, and OAuth authentication

# Demo

[![Demo](./media/thumb.jpg)](https://vimeo.com/1043972987?share=copy#t=0)

# Developing

### Prerequisites

- yt-dlp (will need python3 and ffmpeg)

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

### Architecture

- Frontend: TypeScript, React, Tanstack Router, Tanstack Query, Tailwind CSS
- Backend: Rust, Axum, Tokio, SQLx, AWS S3
- Database: PostgreSQL
- Authentication: Google OAuth2
- Hosting: Self hosted on [homelab](https://github.com/ManeeshWije/homelab)

![Diagram](./media/watch-together-diagram.png)

# TODO

- Don't load the whole video into memory, even though we send by chunks
- Chat system?
