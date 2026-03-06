import { mkdir, unlink } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
export const tempPathFor = async (ext) => {
    const base = '/tmp/bot';
    await mkdir(base, { recursive: true });
    return path.join(base, `${randomUUID()}.${ext}`);
};
export const safeDelete = async (filePath) => {
    await unlink(filePath).catch(() => undefined);
};
export const safeDeleteMany = async (paths) => {
    await Promise.all((paths || []).map((filePath) => safeDelete(filePath)));
};
