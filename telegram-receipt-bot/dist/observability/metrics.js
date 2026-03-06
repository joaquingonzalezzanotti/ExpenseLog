import { Counter, Histogram, Registry } from 'prom-client';
export const registry = new Registry();
export const receiptsReceived = new Counter({ name: 'receipts_received_total', help: 'Receipts received', registers: [registry] });
export const ocrResults = new Counter({ name: 'ocr_results_total', help: 'OCR result by status', labelNames: ['status'], registers: [registry] });
export const parseResults = new Counter({ name: 'parse_results_total', help: 'Parse result by status', labelNames: ['status'], registers: [registry] });
export const draftResults = new Counter({ name: 'draft_results_total', help: 'Draft flow events', labelNames: ['status'], registers: [registry] });
export const stageLatency = new Histogram({
    name: 'receipt_stage_latency_seconds',
    help: 'Latency by stage',
    labelNames: ['stage'],
    buckets: [0.05, 0.1, 0.25, 0.5, 1, 2, 5],
    registers: [registry]
});
