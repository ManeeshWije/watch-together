function sendVideoKey(videoKey) {
    if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "VIDEO_KEY", key: videoKey }));
    } else {
        console.error(
            "WebSocket is not open. Ready state: " + socket.readyState,
        );
    }
}

if (!window.socket && document.getElementById("player")) {
    const socket = new WebSocket("ws://localhost:8080/ws");
    window.socket = socket;

    socket.binaryType = "arraybuffer";
    let videoPlayer = document.getElementById("player");

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
        if (event.data instanceof ArrayBuffer) {
            let blob = new Blob([event.data], { type: "video/mp4" });
            console.log(blob);
            let videoURL = URL.createObjectURL(blob);
            videoPlayer.src = videoURL;
        } else if (event.data === "PLAY") {
            videoPlayer.play();
        } else if (event.data === "PAUSE") {
            videoPlayer.pause();
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
}
