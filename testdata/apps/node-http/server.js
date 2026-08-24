// Minimal HTTP listener with graceful SIGTERM shutdown. Exits 0 on
// SIGTERM after closing the accept socket, so `unitpm stop` observes
// a clean exit and the port is immediately re-bindable.
//
// Port is read from PORT env (default 0 = random free port) so the
// test harness can run multiple instances without colliding.
// Plain 'http' (not 'node:http') so this file works on the older
// node that ships as `nodejs` on ubuntu:22.04 (v12) — the node:
// prefix requires >=16.
const http = require('http');

const port = Number(process.env.PORT || 0);
const server = http.createServer((_req, res) => {
    res.writeHead(200, { 'content-type': 'text/plain' });
    res.end('ok\n');
});

server.listen(port, '127.0.0.1', () => {
    const addr = server.address();
    process.stdout.write(`node-http pid=${process.pid} port=${addr.port}\n`);
});

const shutdown = (sig) => {
    process.stdout.write(`node-http received ${sig}, closing\n`);
    server.close(() => process.exit(0));
    // Hard exit after 5s in case a hung keep-alive blocks close.
    setTimeout(() => process.exit(1), 5000).unref();
};
process.on('SIGTERM', () => shutdown('SIGTERM'));
process.on('SIGINT',  () => shutdown('SIGINT'));
