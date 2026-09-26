const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const { test } = require('node:test');

const context = vm.createContext({});
vm.runInContext(fs.readFileSync(path.join(__dirname, 'sticky_override.js'), 'utf8'), context);
const apply = (config) => JSON.parse(JSON.stringify(context.main(structuredClone(config))));
const proxy = (name) => ({ name, type: 'ss', server: '127.0.0.1', port: 1234 });
const sticky = (config) => config['proxy-groups'].filter((g) => g.name.startsWith('[Sticky] '));

test('creates direct-member selectors without automatic failback groups', () => {
  const result = apply({ proxies: ['US-01', 'JP-01', 'HK-01', '香港-02', 'unknown'].map(proxy) });
  assert.deepEqual(result.proxies.map((p) => p.name), ['US-01', 'JP-01', 'unknown']);
  assert.ok(result['proxy-groups'].every((g) => g.type === 'select'));
  assert.deepEqual(sticky(result).map((g) => g.name), ['[Sticky] 全局', '[Sticky] 美国', '[Sticky] 日本', '[Sticky] 其他地区']);
  for (const group of sticky(result)) {
    assert.equal(group['empty-fallback'], 'REJECT');
    assert.equal(group.interval, undefined);
    assert.ok(group.proxies.every((name) => result.proxies.some((p) => p.name === name)));
  }
  assert.deepEqual(result.rules, ['MATCH,节点选择']);
});

test('dynamic providers retain filters and disable full-pool health checks', () => {
  const result = apply({ 'proxy-providers': { remote: { type: 'http', 'exclude-filter': 'expired', 'health-check': { enable: true, interval: 1 } } } });
  const remote = result['proxy-providers'].remote;
  assert.equal(remote['health-check'].enable, false);
  assert.ok(remote['exclude-filter'].startsWith('expired|'));
  assert.ok(remote['exclude-filter'].includes('hong'));
  assert.equal(sticky(result).length, 12);
  for (const group of sticky(result)) {
    assert.deepEqual(group.use, ['remote']);
    assert.deepEqual(group.proxies, []);
    assert.equal(group['empty-fallback'], 'REJECT');
  }
});

test('rewrites policy targets while preserving direct and reject rules', () => {
  const result = apply({
    proxies: [proxy('US-01')],
    'proxy-groups': [
      { name: 'old', type: 'select', proxies: ['US-01'] },
      { name: 'local', type: 'select', proxies: ['DIRECT'] },
      { name: 'blocked', type: 'select', proxies: ['REJECT'] },
    ],
    rules: ['DOMAIN,a.test,old', 'DOMAIN,b.test,local', 'DOMAIN,c.test,blocked', 'IP-CIDR,1.2.3.0/24,old,no-resolve'],
    dns: { nameserver: ['https://dns.test/dns-query#old&h3'] },
  });
  assert.deepEqual(result.rules, ['DOMAIN,a.test,节点选择', 'DOMAIN,b.test,DIRECT', 'DOMAIN,c.test,REJECT', 'IP-CIDR,1.2.3.0/24,节点选择,no-resolve']);
  assert.deepEqual(result.dns.nameserver, ['https://dns.test/dns-query#节点选择&h3']);
});

test('keeps reserved group names unique against node names', () => {
  const result = apply({ proxies: [proxy('[Sticky] 全局'), proxy('节点选择'), proxy('US-01')] });
  const names = [...result.proxies, ...result['proxy-groups']].map((p) => p.name);
  assert.equal(new Set(names).size, names.length);
  assert.ok(sticky(result).some((g) => g.name === '[Sticky] 全局·'));
});

test('rejects unsupported dependency chains and empty pools', () => {
  assert.throws(() => apply({ proxies: [proxy('HK-01')] }), /没有代理节点/);
  assert.throws(() => apply({
    proxies: [{ ...proxy('US-01'), 'dialer-proxy': 'old' }],
    'proxy-groups': [{ name: 'old', type: 'select', proxies: ['US-01'] }],
  }), /代理链/);
});
