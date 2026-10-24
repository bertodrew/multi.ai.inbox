// A killable forward proxy, used to take the real network away from a browser
// context. Playwright's own offline switch does not stop a service worker or
// the http cache from answering, so a proxy that stops accepting connections is
// the only faithful way to test the offline path.

import http from "node:http";
import net from "node:net";

export type KillableProxy = {
  /** The proxy address to hand to browser.newContext. */
  server: string;
  /** Stops accepting connections and drops every open socket. */
  kill: () => Promise<void>;
};

export async function startKillableProxy(): Promise<KillableProxy> {
  const sockets = new Set<{ destroy: () => void }>();

  const server = http.createServer((request, response) => {
    // The site is https only, so a plain proxied request means something odd.
    response.writeHead(502, { "content-type": "text/plain" });
    response.end(`no plain http through this proxy: ${request.url ?? ""}`);
  });

  server.on("connection", (socket) => {
    sockets.add(socket);
    socket.on("close", () => sockets.delete(socket));
  });

  server.on("connect", (request, clientSocket, head) => {
    sockets.add(clientSocket);
    clientSocket.on("close", () => sockets.delete(clientSocket));
    const target = request.url ?? "";
    const index = target.lastIndexOf(":");
    const host = index === -1 ? target : target.slice(0, index);
    const port = index === -1 ? 443 : Number(target.slice(index + 1));

    const upstream = net.connect(port, host, () => {
      clientSocket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
      if (head.length > 0) upstream.write(head);
      upstream.pipe(clientSocket);
      clientSocket.pipe(upstream);
    });
    sockets.add(upstream);
    upstream.on("close", () => sockets.delete(upstream));
    upstream.on("error", () => clientSocket.destroy());
    clientSocket.on("error", () => upstream.destroy());
  });

  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (address === null || typeof address === "string") throw new Error("the proxy did not bind to a port");

  return {
    server: `http://127.0.0.1:${address.port}`,
    kill: async () => {
      for (const socket of sockets) socket.destroy();
      sockets.clear();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    },
  };
}
