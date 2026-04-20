// Deliberately masks SIGTERM so the supervisor has to fall back to
// SIGKILL after --stop-timeout expires. Used to verify that
// gracefulKill's hard-kill path actually fires instead of hanging.
//
// NOTE: this app is evil on purpose. Never deploy this shape — real
// apps must honour SIGTERM. Tests exist to prove the supervisor
// protects operators even when the supervised app misbehaves.
const http = require('http');

process.on('SIGTERM', () => {
    process.stdout.write('node-ignores-term: ignoring SIGTERM\n');
});

const server = http.createServer((_req, res) => {
    res.end('ok');
});
server.listen(0, '127.0.0.1', () => {
    process.stdout.write(`node-ignores-term pid=${process.pid}\n`);
});
