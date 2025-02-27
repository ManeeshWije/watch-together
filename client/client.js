function sendVideoKey(videoKey) {
    const spinner = document.getElementById("spinner");
    console.log(videoKey)
    if (socket.readyState === WebSocket.OPEN) {
        spinner.style.display = "block";
    } else {
        console.error(
            "WebSocket is not open. Ready state: " + socket.readyState,
        );
    }
}

if (!window.socket && document.getElementById("player")) {
    // const socket = new WebSocket("ws://localhost:8080/ws");
    const socket = new WebSocket("wss://watch.wijeproject.com/ws");
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
        const progressElement = document.getElementById("progress");
        const progressContainer = document.getElementById("progress-container");
        const downloadElement = document.getElementById("download-container");
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
            } else if (message[0] === "Progress") {
                progressContainer.style.display = "block";
                progressElement.value = parseFloat(message[1]);
            } else if (message[0] === "DOWNLOADING") {
                progressContainer.style.display = "none";
                downloadElement.style.display = "block";
            } else if (message[0] === "DOWNLOADED") {
                progressContainer.style.display = "none";
                downloadElement.style.display = "none";
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
        console.log(`Error: ${JSON.stringify(e)}`);
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

    document
        .getElementById("toggle-users")
        .addEventListener("click", function () {
            const usersList = document.getElementById("users-list");
            const button = this;
            const isHidden = usersList.classList.contains("hidden");

            if (isHidden) {
                // Show the users list
                usersList.classList.remove("hidden");
                button.textContent = "Hide Connected Users";

                // Make API call every time we show the list
                htmx.ajax("GET", "/list-users", {
                    target: "#users-list",
                    swap: "innerHTML",
                });
            } else {
                // Hide the users list
                usersList.classList.add("hidden");
                button.textContent = "Show Connected Users";
            }
        });
}
