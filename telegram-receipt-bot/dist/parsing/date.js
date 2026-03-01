export const parseArDateTime = (input) => {
    const normalized = String(input || '').trim().replace(/\s+/g, ' ');
    const numeric = normalized.match(/(\d{2})\/(\d{2})\/(\d{4})(?:.*?(\d{2}):(\d{2}))?/);
    if (numeric) {
        const [, dd, mm, yyyy, hh = '00', min = '00'] = numeric;
        return `${yyyy}-${mm}-${dd}T${hh}:${min}:00-03:00`;
    }
    const monthMap = {
        enero: '01',
        febrero: '02',
        marzo: '03',
        abril: '04',
        mayo: '05',
        junio: '06',
        julio: '07',
        agosto: '08',
        septiembre: '09',
        setiembre: '09',
        octubre: '10',
        noviembre: '11',
        diciembre: '12'
    };
    const textual = normalized
        .toLowerCase()
        .match(/(\d{1,2})\s+de\s+([a-záéíóú]+)\s+de\s+(\d{4})(?:.*?(\d{1,2}):(\d{2}))?/);
    if (textual) {
        const [, d, monthText, yyyy, h = '00', min = '00'] = textual;
        const mm = monthMap[monthText.normalize('NFD').replace(/[\u0300-\u036f]/g, '')];
        if (!mm)
            return undefined;
        const dd = d.padStart(2, '0');
        const hh = h.padStart(2, '0');
        return `${yyyy}-${mm}-${dd}T${hh}:${min}:00-03:00`;
    }
    return undefined;
};
