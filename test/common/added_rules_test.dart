import 'dart:convert';

import 'package:drift/native.dart';
import 'package:fl_clash/common/added_rules.dart';
import 'package:fl_clash/database/database.dart' as db;
import 'package:fl_clash/models/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('added rule format', () {
    test('reads plain text with BOM, comments and Windows line endings', () {
      final rules = decodeAddedRules(
        '\uFEFF# rules\r\n\r\n DOMAIN, a.com, DIRECT \r\n'
        'IP-CIDR,192.0.2.0/24,Proxy,src,no-resolve\r\n',
      );

      expect(rules.map((rule) => rule.rawValue), [
        'DOMAIN,a.com,DIRECT',
        'IP-CIDR,192.0.2.0/24,Proxy,src,no-resolve',
      ]);
    });

    test('reads YAML rule maps, bare lists and full profile rules', () {
      for (final content in [
        'rules:\n  - DOMAIN,a.com,DIRECT\n',
        '- DOMAIN,a.com,DIRECT\n',
        'rules: ["DOMAIN,a.com,DIRECT"]',
        '---\nproxies: []\nrules:\n  - DOMAIN,a.com,DIRECT\n',
        'proxies: []\nrules:\n  - DOMAIN,a.com,DIRECT\n',
      ]) {
        expect(
          decodeAddedRules(content).single.rawValue,
          'DOMAIN,a.com,DIRECT',
        );
      }
    });

    test('preserves comma payloads, Unicode targets and parameters', () {
      const values = [
        'DOMAIN,a.com,香港',
        r'DOMAIN-REGEX,^a{1,3}\.com$,Proxy',
        'AND,((NETWORK,TCP),(DST-PORT,443)),Proxy',
        'NOT,((DOMAIN,a.com)),REJECT',
        'GEOIP,CN,DIRECT,no-resolve',
        'IP-CIDR,192.0.2.0/24,DIRECT,src,no-resolve',
      ];
      final rules = values.map((value) => Rule.parse(value)).toList();
      final content = utf8.decode(encodeAddedRules(rules));

      expect(decodeAddedRules(content).map((rule) => rule.rawValue), values);
    });

    test('rejects empty, malformed and unsupported input', () {
      for (final content in [
        '',
        '# no rules\n',
        'rules: []',
        'rules: null',
        'proxies: []',
        'rules: [1]',
        'rules: [',
        'UNKNOWN,a.com,DIRECT',
        'DOMAIN,a.com',
        'DOMAIN,,DIRECT',
        'DOMAIN,a.com,',
        'DOMAIN,a.com,DIRECT,extra',
        'IP-CIDR,192.0.2.0/24,DIRECT,typo',
        'IP-CIDR,192.0.2.0/24,DIRECT,src,src',
        'MATCH,DIRECT',
        'RULE-SET,remote,DIRECT',
        'SUB-RULE,(NETWORK,TCP),sub',
      ]) {
        expect(
          () => decodeAddedRules(content),
          throwsA(isA<FormatException>()),
          reason: content,
        );
      }
    });

    test('reports the invalid entry instead of dropping it', () {
      expect(
        () => decodeAddedRules('DOMAIN,a.com,DIRECT\nBOGUS,b.com,DIRECT'),
        throwsA(
          isA<FormatException>().having(
            (error) => error.message,
            'message',
            contains('rule 2'),
          ),
        ),
      );
    });
  });

  group('added rule import', () {
    late db.Database database;

    setUp(() async {
      database = db.Database(NativeDatabase.memory());
      db.database = database;
      await database.rulesDao.queryGlobalAddedRules().get();
    });

    tearDown(() async {
      await database.close();
    });

    test(
      'appends in file order and preserves existing IDs and order',
      () async {
        const existing = Rule(
          id: 1,
          content: 'existing.com',
          ruleTarget: 'DIRECT',
          order: 'a0',
        );
        await database.rulesDao.putGlobalRule(existing);
        final imported = decodeAddedRules(
          'DOMAIN,a.com,DIRECT\nDOMAIN,b.com,Proxy',
        );

        expect(await importAddedRules(imported), 2);
        final stored = await database.rulesDao.queryGlobalAddedRules().get();
        expect(stored.first, existing);
        expect(stored.map((rule) => rule.content), [
          'existing.com',
          'a.com',
          'b.com',
        ]);
        expect(stored.map((rule) => rule.id).toSet(), hasLength(3));
        final importedIds = imported.map((rule) => rule.id).toSet()..add(1);
        expect(
          stored.skip(1).every((rule) => !importedIds.contains(rule.id)),
          isTrue,
        );
      },
    );

    test('skips duplicates but keeps distinct targets and flags', () async {
      final imported = decodeAddedRules(
        'DOMAIN,a.com,DIRECT\nDOMAIN,a.com,DIRECT\nDOMAIN,a.com,Proxy\n'
        'IP-CIDR,192.0.2.0/24,DIRECT\n'
        'IP-CIDR,192.0.2.0/24,DIRECT,no-resolve',
      );

      expect(await importAddedRules(imported), 4);
      final before = await database.rulesDao.queryGlobalAddedRules().get();
      expect(await importAddedRules(imported), 0);
      expect(await database.rulesDao.queryGlobalAddedRules().get(), before);
    });

    test('does not change profile-specific rules', () async {
      await database.profilesDao.putAll([
        const Profile(id: 1, autoUpdateDuration: Duration.zero).toCompanion(),
      ]);
      const profileRule = Rule(
        id: 2,
        content: 'profile.com',
        ruleTarget: 'DIRECT',
        order: 'a0',
      );
      await database.rulesDao.putProfileAddedRule(1, profileRule);

      await importAddedRules(decodeAddedRules('DOMAIN,a.com,DIRECT'));

      expect(await database.rulesDao.queryProfileAddedRules(1).get(), [
        profileRule,
      ]);
    });

    test('rolls back the entire import when a later write fails', () async {
      await database.customStatement('''
CREATE TRIGGER reject_bad_rule BEFORE INSERT ON rules
WHEN NEW.content = 'bad.com'
BEGIN
  SELECT RAISE(ABORT, 'write failed');
END
''');

      await expectLater(
        importAddedRules(
          decodeAddedRules('DOMAIN,a.com,DIRECT\nDOMAIN,bad.com,DIRECT'),
        ),
        throwsA(isA<Exception>()),
      );

      expect(await database.rulesDao.queryGlobalAddedRules().get(), isEmpty);
      expect(await database.select(database.rules).get(), isEmpty);
    });
  });
}
