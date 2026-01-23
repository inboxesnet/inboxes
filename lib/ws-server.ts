import { WebSocketServer, WebSocket } from "ws";
import { IncomingMessage } from "http";
import { jwtVerify } from "jose";

interface ConnectedClient {
  userId: string;
  orgId: string;
  socket: WebSocket;
}

const clients: Map<string, Set<ConnectedClient>> = new Map();

function getSecret() {
  const secret = process.env.SESSION_SECRET;
  if (!secret) throw new Error("SESSION_SECRET is not set");
  return new TextEncoder().encode(secret);
}

function parseCookies(cookieHeader: string): Record<string, string> {
  const cookies: Record<string, string> = {};
  cookieHeader.split(";").forEach((cookie) => {
    const [name, ...rest] = cookie.trim().split("=");
    if (name) {
      cookies[name] = decodeURIComponent(rest.join("="));
    }
  });
  return cookies;
}

async function authenticateConnection(
  req: IncomingMessage
): Promise<{ userId: string; orgId: string } | null> {
  const cookieHeader = req.headers.cookie;
  if (!cookieHeader) return null;

  const cookies = parseCookies(cookieHeader);
  const token = cookies["session"];
  if (!token) return null;

  try {
    const { payload } = await jwtVerify(token, getSecret());
    const userId = payload.user_id as string;
    const orgId = payload.org_id as string;
    if (!userId || !orgId) return null;
    return { userId, orgId };
  } catch {
    return null;
  }
}

function addClient(client: ConnectedClient) {
  const existing = clients.get(client.userId);
  if (existing) {
    existing.add(client);
  } else {
    clients.set(client.userId, new Set([client]));
  }
}

function removeClient(client: ConnectedClient) {
  const existing = clients.get(client.userId);
  if (existing) {
    existing.delete(client);
    if (existing.size === 0) {
      clients.delete(client.userId);
    }
  }
}

export function notifyUser(
  userId: string,
  event: string,
  payload: unknown
): void {
  const userClients = clients.get(userId);
  if (!userClients) return;

  const message = JSON.stringify({ event, payload });
  userClients.forEach((client) => {
    if (client.socket.readyState === WebSocket.OPEN) {
      client.socket.send(message);
    }
  });
}

export function createWebSocketServer(port: number = 3001): WebSocketServer {
  const wss = new WebSocketServer({ port });

  wss.on("connection", async (socket: WebSocket, req: IncomingMessage) => {
    const auth = await authenticateConnection(req);
    if (!auth) {
      socket.close(4001, "Unauthorized");
      return;
    }

    const client: ConnectedClient = {
      userId: auth.userId,
      orgId: auth.orgId,
      socket,
    };

    addClient(client);

    socket.send(JSON.stringify({ event: "connected", payload: null }));

    socket.on("close", () => {
      removeClient(client);
    });

    socket.on("error", () => {
      removeClient(client);
    });

    // Keep-alive ping every 30 seconds
    const pingInterval = setInterval(() => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.ping();
      } else {
        clearInterval(pingInterval);
      }
    }, 30000);

    socket.on("close", () => {
      clearInterval(pingInterval);
    });
  });

  return wss;
}

export { clients };
