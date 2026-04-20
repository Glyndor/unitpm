<?php
// Long-running PHP worker with SIGTERM handling via pcntl. Emits a
// heartbeat line each second and exits 0 on SIGTERM / SIGINT so the
// supervisor sees a clean graceful stop — mirrors python-worker and
// ruby-worker so any runtime-specific regression stands out.

declare(strict_types=1);

pcntl_async_signals(true);
pcntl_signal(SIGTERM, static function (): void {
    fwrite(STDOUT, "php-worker received SIGTERM, exiting\n");
    exit(0);
});
pcntl_signal(SIGINT, static function (): void {
    fwrite(STDOUT, "php-worker received SIGINT, exiting\n");
    exit(0);
});

fwrite(STDOUT, sprintf("php-worker pid=%d\n", getmypid()));
$tick = 0;
while (true) {
    fwrite(STDOUT, sprintf("php-worker tick=%d\n", $tick));
    $tick++;
    sleep(1);
}
