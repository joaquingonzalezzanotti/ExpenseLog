import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { config } from '../config.js';
import { ocrResults, stageLatency } from '../observability/metrics.js';
const execFileAsync = promisify(execFile);
export const ocrImage = async (filePath) => {
    const stopTimer = stageLatency.startTimer({ stage: 'ocr' });
    try {
        const { stdout } = await execFileAsync('tesseract', [filePath, 'stdout', '-l', config.ocrLangs]);
        ocrResults.inc({ status: 'ok' });
        return stdout;
    }
    catch (error) {
        ocrResults.inc({ status: 'fail' });
        throw error;
    }
    finally {
        stopTimer();
    }
};
