import { config } from '../config.js';
import { logger } from '../observability/logger.js';
import { ocrImage } from './ocr.js';
import { extractPdfText } from './pdf.js';
import { parseModoReceipt } from '../parsing/modo.js';
import { parseGaliciaReceipt } from '../parsing/galicia.js';
import { parseTransferReceipt } from '../parsing/transfer.js';
import { parseGenericReceipt } from '../parsing/generic.js';
import { safeDeleteMany } from './file.js';
import { stageLatency } from '../observability/metrics.js';
export const processReceipt = async (input) => {
    let text = '';
    const renderedImagePaths = [];
    const stopPipeline = stageLatency.startTimer({ stage: 'pipeline_total' });
    try {
        if (input.fileType === 'image') {
            text = await ocrImage(input.filePath);
        }
        else {
            const extraction = await extractPdfText(input.filePath);
            text = extraction.text;
            renderedImagePaths.push(...extraction.renderedImagePaths);
            if (text.trim().length < 30 && renderedImagePaths.length > 0) {
                for (const renderedImagePath of renderedImagePaths.slice(0, config.pdfRenderMaxPages)) {
                    text += `\n${await ocrImage(renderedImagePath)}`;
                }
            }
        }
        logger.info('receipt_text_processed', { excerpt: logger.sanitizeText(text) });
        const stopParse = stageLatency.startTimer({ stage: 'parse' });
        try {
            if ((/Transferencia de/i.test(text) && /CBU\/CVU/i.test(text)) || (/MODO/i.test(text) && /(Pagaste a|Ref\.?)/i.test(text))) {
                return parseModoReceipt(text, config.myAliases, input.telegramMeta);
            }
            if (/Galicia/i.test(text) && /(Transferencia enviada|N[°ºo]\s+de operaci[oó]n)/i.test(text)) {
                return parseGaliciaReceipt(text, config.myAliases, input.telegramMeta);
            }
            if (/Comprobante de transferencia/i.test(text) || /(Mercado Pago|Fecha de ejecuci[oó]n|Destinatario)/i.test(text)) {
                return parseTransferReceipt(text, config.myAliases, input.telegramMeta);
            }
            return parseGenericReceipt(text, input.telegramMeta);
        }
        finally {
            stopParse();
        }
    }
    finally {
        await safeDeleteMany(renderedImagePaths);
        stopPipeline();
    }
};
