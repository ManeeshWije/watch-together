function sendVideoKey(videoKey) {
    const spinner = document.getElementById("spinner");
    if (socket.readyState === WebSocket.OPEN) {
        spinner.style.display = "block";
        socket.send(JSON.stringify({ type: "VIDEO_KEY", key: videoKey }));
    } else {
        console.error(
            "WebSocket is not open. Ready state: " + socket.readyState,
        );
    }
}

if (!window.socket && document.getElementById("player")) {
    // const socket = new WebSocket("ws://localhost:8080/ws");
    const socket = new WebSocket("ws://watch-together.up.railway.app/ws");
    window.socket = socket;

    socket.binaryType = "arraybuffer";
    let videoPlayer = document.getElementById("player");
    const spinner = document.getElementById("spinner");
    let isSyncing = false;

    if (!videoPlayer) {
        console.warn(
            "videoPlayer element not found, but probably because you haven't clicked a video yet",
        );
    }

    socket.onopen = (_) => {
        console.log("Connected");
    };

    socket.onclose = (_) => {
        console.log("Disconnected");
    };

    socket.onmessage = (event) => {
        console.log(event);
        if (typeof event.data === "string" || event.data instanceof String) {
            const message = event.data.split(":");
            if (message[0] === "TIMESTAMP") {
                if (!isSyncing) {
                    isSyncing = true;
                    const timestamp = parseFloat(message[1]);
                    videoPlayer.currentTime = timestamp / 1000;
                    setTimeout(() => {
                        isSyncing = false;
                    }, 500); // Re-enable after a short delay
                }
            } else if (message[0] === "PLAY") {
                videoPlayer.play();
            } else if (message[0] === "PAUSE") {
                videoPlayer.pause();
            }
        } else if (event.data instanceof ArrayBuffer) {
            spinner.style.display = "none";
            let blob = new Blob([event.data], { type: "video/mp4" });
            console.log(blob);
            let videoURL = URL.createObjectURL(blob);
            videoPlayer.src = videoURL;
        } else {
            console.error("WTF");
        }
    };

    socket.onerror = (e) => {
        console.log(`Error: ${e}`);
    };

    videoPlayer.onplay = () => {
        socket.send("PLAY");
    };

    videoPlayer.onpause = () => {
        socket.send("PAUSE");
    };

    videoPlayer.onseeked = () => {
        if (!isSyncing) {
            const timestampInMs = videoPlayer.currentTime * 1000;
            socket.send(`TIMESTAMP:${timestampInMs}`);
        }
    };
}
