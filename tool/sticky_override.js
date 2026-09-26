// Requires the Sticky-enabled FlClash Core; ordinary Selectors do not fail over.
function main(config) {
  // 字母缩写限定边界，避免把 Austria 中的 us 等误认为地区代码。
  const code = (s) => "(^|[^a-zA-Z])(" + s + ")([^a-zA-Z]|$)";
  const hk = "香港|港|🇭🇰|hong[ _-]*kong|" + code("HK|HKG");
  const hkRE = new RegExp(hk, "i");
  const regions = [
    ["美国", "🇺🇸|美国|美國|洛杉矶|洛杉磯|纽约|紐約|西雅图|西雅圖|圣何塞|聖何塞|united[ _-]*states|america|los[ _-]*angeles|seattle|san[ _-]*jose|" + code("US|USA")],
    ["新加坡", "🇸🇬|新加坡|狮城|獅城|singapore|" + code("SG|SGP")],
    ["日本", "🇯🇵|日本|东京|東京|大阪|japan|tokyo|osaka|" + code("JP|JPN")],
    ["台湾", "🇹🇼|台湾|臺灣|台灣|台北|臺北|taiwan|taipei|" + code("TW|TWN")],
    ["韩国", "🇰🇷|韩国|韓國|首尔|首爾|釜山|korea|seoul|" + code("KR|KOR")],
    ["英国", "🇬🇧|英国|英國|伦敦|倫敦|united[ _-]*kingdom|britain|london|" + code("UK|GB|GBR")],
    ["德国", "🇩🇪|德国|德國|法兰克福|法蘭克福|germany|frankfurt|" + code("DE|DEU")],
    ["法国", "🇫🇷|法国|法國|巴黎|france|paris|" + code("FR|FRA")],
    ["加拿大", "🇨🇦|加拿大|多伦多|多倫多|温哥华|溫哥華|canada|toronto|vancouver|" + code("CA|CAN")],
    ["澳大利亚", "🇦🇺|澳大利亚|澳大利亞|澳洲|悉尼|墨尔本|墨爾本|australia|sydney|melbourne|" + code("AU|AUS")],
  ];
  const oldGroups = config["proxy-groups"] || [];
  const oldProxies = config.proxies || [];
  const providers = config["proxy-providers"] || {};
  const providerNames = Object.keys(providers);
  const originalGroups = new Map(oldGroups.map((g) => [g.name, g]));
  const originalProxies = new Map(oldProxies.map((p) => [p.name, p]));
  const builtins = new Set(["DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"]);

  // 纯直连/拒绝组保持语义；混合策略组改由统一的自动策略管理。
  function fixedAction(name, seen) {
    if (builtins.has(name)) return name;
    const proxy = originalProxies.get(name);
    if (proxy && proxy.type === "direct") return "DIRECT";
    if (proxy && proxy.type === "reject") return "REJECT";
    const g = originalGroups.get(name);
    if (!g || seen.has(name) || (g.use || []).length ||
        g["include-all"] || g["include-all-proxies"] || g["include-all-providers"]) return null;
    const members = g.proxies || [];
    if (!members.length) return null;
    const next = new Set(seen);
    next.add(name);
    const actions = members.map((n) => fixedAction(n, next));
    return actions[0] && actions.every((a) => a === actions[0]) ? actions[0] : null;
  }

  const proxies = oldProxies.filter((p) => !hkRE.test(p.name || ""));
  const candidates = proxies.filter((p) =>
    !["direct", "reject", "reject-drop", "dns", "pass", "pass-rule", "rematch", "compatible"].includes(String(p.type).toLowerCase())
  );
  if (!candidates.length && !providerNames.length) {
    throw new Error("过滤香港后没有代理节点，请检查订阅与节点命名。");
  }
  config.proxies = proxies;

  const used = new Set(proxies.map((p) => p.name).concat(providerNames));
  function unique(name) {
    let result = name;
    while (used.has(result)) result += "·";
    used.add(result);
    return result;
  }
  const entry = unique("节点选择");
  const globalSticky = unique("[Sticky] 全局");
  const manual = unique("手动节点");
  const targetMap = new Map();
  oldGroups.forEach((g) => targetMap.set(g.name, fixedAction(g.name, new Set()) || entry));
  oldProxies.forEach((p) => {
    if (hkRE.test(p.name || "")) targetMap.set(p.name, entry);
  });
  const mapTarget = (name) => targetMap.get(name) || name;

  providerNames.forEach((name) => {
    const p = providers[name];
    const previous = p["exclude-filter"];
    // scoped (?i:...) 不改变原有正则的大小写语义。
    p["exclude-filter"] = (previous ? previous + "|" : "") + "(?i:" + hk + ")";
    p["health-check"] = {
      ...(p["health-check"] || {}),
      enable: false,
    };
    if (p.proxy) p.proxy = mapTarget(p.proxy);
    function checkProviderChain(node) {
      const via = node && node["dialer-proxy"];
      if (via && (originalGroups.has(via) || hkRE.test(via))) {
        throw new Error("provider 节点依赖旧策略组或香港前置代理，需先单独适配代理链：" + name);
      }
    }
    (p.payload || []).forEach(checkProviderChain);
    if (p.override && p.override["dialer-proxy"]) {
      const via = p.override["dialer-proxy"];
      if (originalGroups.has(via) || hkRE.test(via)) {
        throw new Error("provider 使用了旧策略组或香港前置代理，需先单独适配代理链：" + name);
      }
    }
  });

  // 代理链依赖不能随意改为自动组，否则可能形成节点拨号递归。
  proxies.forEach((p) => {
    const via = p["dialer-proxy"];
    if (via && (originalGroups.has(via) || hkRE.test(via))) {
      throw new Error("节点依赖旧策略组或已删除的香港前置代理，需先单独适配代理链：" + p.name);
    }
  });

  function pool(names, filter, exclude) {
    const result = { proxies: names, "empty-fallback": "REJECT" };
    if (providerNames.length) result.use = providerNames.slice();
    if (filter) result.filter = "(?i:" + filter + ")";
    result["exclude-filter"] = "(?i:" + hk + (exclude ? "|" + exclude : "") + ")";
    result["exclude-type"] = "Direct|Reject|RejectDrop|Dns|Pass|PassRule|Rematch|Compatible";
    return result;
  }
  const groups = [];
  const regionNames = [];
  let claimed = "";
  const claimedNodes = new Set();
  regions.forEach(([label, pattern]) => {
    const re = new RegExp(pattern, "i");
    const names = candidates.filter((p) => !claimedNodes.has(p.name) && re.test(p.name))
      .map((p) => p.name);
    names.forEach((n) => claimedNodes.add(n));
    if (names.length || providerNames.length) {
      const name = unique("[Sticky] " + label);
      regionNames.push(name);
      groups.push({
        name, type: "select",
        ...pool(names, pattern, claimed),
      });
    }
    claimed += (claimed ? "|" : "") + pattern;
  });
  const other = candidates.filter((p) => !claimedNodes.has(p.name)).map((p) => p.name);
  if (other.length || providerNames.length) {
    const name = unique("[Sticky] 其他地区");
    regionNames.push(name);
    groups.push({
      name, type: "select",
      ...pool(other, "", claimed),
    });
  }
  const allNames = candidates.map((p) => p.name);
  config["proxy-groups"] = [
    { name: entry, type: "select", proxies: [globalSticky, ...regionNames, manual] },
    { name: globalSticky, type: "select", ...pool(allNames) },
    ...groups,
    { name: manual, type: "select", ...pool(allNames) },
  ];

  function rewriteRule(rule) {
    if (typeof rule !== "string") return rule;
    const parts = rule.split(",");
    if (parts[0].trim().toUpperCase() === "SUB-RULE") return rule;
    let index = parts.length - 1;
    while (index > 0 && ["no-resolve", "src"].includes(parts[index].trim())) index--;
    if (index > 0) parts[index] = mapTarget(parts[index].trim());
    return parts.join(",");
  }
  config.rules = Array.isArray(config.rules) && config.rules.length
    ? config.rules.map(rewriteRule)
    : ["MATCH," + entry];
  Object.keys(config["sub-rules"] || {}).forEach((name) => {
    config["sub-rules"][name] = config["sub-rules"][name].map(rewriteRule);
  });
  Object.values(config["rule-providers"] || {}).forEach((p) => {
    if (p.proxy) p.proxy = mapTarget(p.proxy);
  });
  (config.tunnels || []).forEach((t) => { if (t.proxy) t.proxy = mapTarget(t.proxy); });
  (config.listeners || []).forEach((l) => { if (l.proxy) l.proxy = mapTarget(l.proxy); });

  function rewriteDNS(value) {
    if (typeof value === "string") {
      const hash = value.indexOf("#");
      if (hash < 0) return value;
      const suffix = value.slice(hash + 1).split("&");
      suffix[0] = mapTarget(suffix[0]);
      return value.slice(0, hash + 1) + suffix.join("&");
    }
    if (Array.isArray(value)) return value.map(rewriteDNS);
    if (value && typeof value === "object") {
      Object.keys(value).forEach((key) => { value[key] = rewriteDNS(value[key]); });
    }
    return value;
  }
  if (config.dns) config.dns = rewriteDNS(config.dns);
  return config;
}
