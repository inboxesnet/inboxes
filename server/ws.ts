import { createServer, IncomingMessage, ServerResponse } from "http";
import { createWebSocketServer, notifyUser } from "../lib/ws-server";

const wsPort = parseInt(process.env.WS_PORT || "3001", 10);
const httpPort = parseInt(process.env.WS_HTTP_PORT || "3002", 10);

const wss = createWebSocketServer(wsPort);
console.log(`WebSocket server running on port ${wsPort}`);

// Internal HTTP API for notify requests from Next.js process
const httpServer = createServer(
  async (req: IncomingMessage, res: ServerResponse) => {
    if (req.method === "POST" && req.url === "/notify") {
      let body = "";
      req.on("data", (chunk: Buffer) => {
        body += chunk.toString();
      });
      req.on("end", () => {
        try {
          const { userId, event, payload } = JSON.parse(body);
          if (!userId || !event) {
            res.writeHead(400);
            res.end("Missing userId or event");
            return;
          }
          notifyUser(userId, event, payload);
          res.writeHead(200);
          res.end("OK");
        } catch {
          res.writeHead(400);
          res.end("Invalid JSON");
        }
      });
    } else {
      res.writeHead(404);
      res.end("Not found");
    }
  }
);

httpServer.listen(httpPort, () => {
  console.log(`WebSocket internal HTTP API running on port ${httpPort}`);
});

process.on("SIGTERM", () => {
  wss.close();
  httpServer.close(() => {
    console.log("Servers closed");
    process.exit(0);
  });
});

process.on("SIGINT", () => {
  wss.close();
  httpServer.close(() => {
    console.log("Servers closed");
    process.exit(0);
  });
});
