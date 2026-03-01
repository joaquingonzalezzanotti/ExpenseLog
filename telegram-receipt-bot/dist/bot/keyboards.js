import { Markup } from 'telegraf';
export const mainDecisionKeyboard = () => Markup.inlineKeyboard([
    [Markup.button.callback('Confirmar', 'confirm')],
    [Markup.button.callback('Corregir datos', 'fix_menu')],
    [Markup.button.callback('Descartar', 'reject')]
]);
export const fixMenuKeyboard = () => Markup.inlineKeyboard([
    [Markup.button.callback('Cambiar monto', 'fix_amount'), Markup.button.callback('Cambiar fecha y hora', 'fix_datetime')],
    [Markup.button.callback('Cambiar contraparte', 'fix_counterparty'), Markup.button.callback('Cambiar tipo', 'fix_type')],
    [Markup.button.callback('Cambiar motivo', 'fix_motive')],
    [Markup.button.callback('Volver', 'back_summary')]
]);
export const dedupeKeyboard = () => Markup.inlineKeyboard([
    [Markup.button.callback('Crear de todos modos', 'dedupe_create_anyway')],
    [Markup.button.callback('Cancelar carga', 'dedupe_cancel')]
]);
