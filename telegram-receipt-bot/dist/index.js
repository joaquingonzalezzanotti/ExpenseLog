import http from 'node:http';
import { buildBot } from './bot/bot.js';
import { config } from './config.js';
import { logger } from './observability/logger.js';
import { registry } from './observability/metrics.js';
const bot = buildBot();
let stopping = false;
let launchRetryTimer = null;
const parsePathname = (rawUrl) => {
    try {
        return new URL(rawUrl || '/', 'http://127.0.0.1').pathname;
    }
    catch {
        return '/';
    }
};
const isTelegramConflict409 = (error) => {
    const maybe = error;
    return maybe?.response?.error_code === 409;
};
const scheduleBotLaunch = (delayMs) => {
    if (stopping)
        return;
    if (launchRetryTimer)
        clearTimeout(launchRetryTimer);
    launchRetryTimer = setTimeout(() => {
        launchRetryTimer = null;
        void launchBotWithRetry();
    }, delayMs);
};
const launchBotWithRetry = async () => {
    if (stopping)
        return;
    try {
        // Ensure long-polling starts from a clean state after Railway restarts.
        await bot.telegram.deleteWebhook({ drop_pending_updates: true });
        await bot.launch({ dropPendingUpdates: true });
        logger.info('telegram_bot_started');
    }
    catch (error) {
        const retryMs = isTelegramConflict409(error) ? 10_000 : 15_000;
        logger.error('telegram_bot_launch_failed', { retryMs, error });
        scheduleBotLaunch(retryMs);
    }
};
const server = http.createServer(async (req, res) => {
    const pathname = parsePathname(req.url);
    if (pathname === '/metrics') {
        res.setHeader('Content-Type', registry.contentType);
        res.end(await registry.metrics());
        return;
    }
    if (pathname === '/' || pathname === '/healthz' || pathname === '/health') {
        res.statusCode = 200;
        res.end('ok');
        return;
    }
    res.statusCode = 404;
    res.end('not found');
});
server.listen(config.port, () => logger.info('metrics_server_started', { port: config.port }));
void launchBotWithRetry();
const shutdown = (signal) => {
    if (stopping)
        return;
    stopping = true;
    logger.info('shutdown_signal_received', { signal });
    if (launchRetryTimer)
        clearTimeout(launchRetryTimer);
    try {
        bot.stop(signal);
    }
    catch (error) {
        logger.warn('telegram_bot_stop_failed', { signal, error });
    }
    server.close(() => {
        logger.info('metrics_server_stopped', { signal });
        process.exit(0);
    });
    setTimeout(() => process.exit(0), 8_000).unref();
};
process.once('SIGINT', () => shutdown('SIGINT'));
process.once('SIGTERM', () => shutdown('SIGTERM'));
