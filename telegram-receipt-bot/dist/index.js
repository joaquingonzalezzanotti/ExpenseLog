import http from 'node:http';
import { buildBot } from './bot/bot.js';
import { config } from './config.js';
import { logger } from './observability/logger.js';
import { registry } from './observability/metrics.js';
const bot = buildBot();
let stopping = false;
let launchRetryTimer = null;
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
        await bot.launch();
        logger.info('telegram_bot_started');
    }
    catch (error) {
        const retryMs = isTelegramConflict409(error) ? 10_000 : 15_000;
        logger.error('telegram_bot_launch_failed', { retryMs, error });
        scheduleBotLaunch(retryMs);
    }
};
const server = http.createServer(async (req, res) => {
    if (req.url === '/metrics') {
        res.setHeader('Content-Type', registry.contentType);
        res.end(await registry.metrics());
        return;
    }
    if (req.url === '/healthz') {
        res.statusCode = 200;
        res.end('ok');
        return;
    }
    res.statusCode = 404;
    res.end('not found');
});
server.listen(config.port, () => logger.info('metrics_server_started', { port: config.port }));
void launchBotWithRetry();
process.once('SIGINT', () => {
    stopping = true;
    if (launchRetryTimer)
        clearTimeout(launchRetryTimer);
    bot.stop('SIGINT');
});
process.once('SIGTERM', () => {
    stopping = true;
    if (launchRetryTimer)
        clearTimeout(launchRetryTimer);
    bot.stop('SIGTERM');
});
