import { readFile } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import pdfParse from 'pdf-parse';
import { config } from '../config.js';
import { stageLatency } from '../observability/metrics.js';
const execFileAsync = promisify(execFile);
export const extractPdfText = async (filePath) => {
    const stopTimer = stageLatency.startTimer({ stage: 'pdf_extract' });
    try {
        const data = await readFile(filePath);
        const parsed = await pdfParse(data);
        if (parsed.text && parsed.text.trim().length > 30) {
            return { text: parsed.text, renderedImagePaths: [] };
        }
        const pages = Math.max(1, config.pdfRenderMaxPages);
        const prefix = `${filePath}-render`;
        await execFileAsync('pdftoppm', ['-png', '-f', '1', '-l', String(pages), filePath, prefix]);
        const renderedImagePaths = Array.from({ length: pages }, (_, idx) => `${prefix}-${idx + 1}.png`);
        return { text: '', renderedImagePaths };
    }
    finally {
        stopTimer();
    }
};
