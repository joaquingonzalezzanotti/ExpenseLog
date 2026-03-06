const resolveValue = (receipt, field) => {
    if (field === 'account_from.bank')
        return receipt.account_from?.bank;
    if (field === 'account_to.bank')
        return receipt.account_to?.bank;
    return receipt[field]?.toString();
};
const evaluate = (actual, op, value) => {
    if (!actual)
        return false;
    switch (op) {
        case 'equals': return actual.toLowerCase() === String(value).toLowerCase();
        case 'contains': return actual.toLowerCase().includes(String(value).toLowerCase());
        case 'regex': return new RegExp(String(value), 'i').test(actual);
        case 'in': return Array.isArray(value) && value.map((v) => v.toLowerCase()).includes(actual.toLowerCase());
        default: return false;
    }
};
const opWeight = { equals: 1, in: 1, contains: 2, regex: 3 };
export const applyRules = (receipt, rules) => {
    const sorted = [...rules].filter((r) => r.enabled).sort((a, b) => {
        const w = opWeight[a.when.op] - opWeight[b.when.op];
        if (w !== 0)
            return w;
        return a.priority - b.priority;
    });
    const output = { tags: [] };
    const setIfMissing = (key, value) => {
        if (!value)
            return;
        if (output[key])
            return;
        output[key] = value;
    };
    for (const rule of sorted) {
        const actual = resolveValue(receipt, rule.when.field);
        if (!evaluate(actual, rule.when.op, rule.when.value))
            continue;
        if (rule.then.set) {
            setIfMissing('category', rule.then.set.category);
            setIfMissing('account', rule.then.set.account);
            setIfMissing('notes_prefix', rule.then.set.notes_prefix);
        }
        if (rule.then.tags_add)
            output.tags = [...new Set([...(output.tags ?? []), ...rule.then.tags_add])];
    }
    return output;
};
