import 'dart:convert';
import 'dart:typed_data';

import 'package:fl_clash/common/indexing.dart';
import 'package:fl_clash/common/snowflake.dart';
import 'package:fl_clash/common/yaml.dart';
import 'package:fl_clash/database/database.dart';
import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/models/models.dart';
import 'package:yaml/yaml.dart';

List<Rule> decodeAddedRules(String content) {
  final lines = const LineSplitter().convert(
    content.replaceFirst('\uFEFF', ''),
  );
  final entries = lines
      .map((line) => line.trim())
      .where((line) => line.isNotEmpty && !line.startsWith('#'))
      .toList();
  if (entries.isEmpty) {
    throw const FormatException('No rules found');
  }
  final firstType = entries.first.split(',').first.trim().toUpperCase();
  final isPlainText = RuleAction.values.any(
    (action) => action.value == firstType,
  );
  final Object? values;
  if (isPlainText) {
    values = entries;
  } else {
    final document = loadYaml(content);
    values = document is Map ? document['rules'] : document;
  }
  if (values is! List || values.isEmpty) {
    throw const FormatException('Expected a non-empty rules list');
  }
  return [
    for (var index = 0; index < values.length; index++)
      _decodeAddedRule(values[index], index + 1),
  ];
}

Rule _decodeAddedRule(Object? value, int position) {
  if (value is! String) {
    throw FormatException('Rule $position must be a string');
  }
  final fields = value.split(',').map((field) => field.trim()).toList();
  final type = fields.first.toUpperCase();
  final action = RuleAction.addedRuleActions
      .where((action) => action.value == type)
      .firstOrNull;
  if (action == null) {
    throw FormatException('Unsupported rule type at rule $position: $type');
  }
  final rule = Rule.parse(value);
  if (rule.realContent == null || rule.realTarget == null) {
    throw FormatException('Missing content or target at rule $position');
  }
  if (!action.hasCommaPayload) {
    final params = fields.skip(3).toList();
    if (fields.length < 3 ||
        (!action.hasParams && params.isNotEmpty) ||
        params.any((param) => param != 'src' && param != 'no-resolve') ||
        params.toSet().length != params.length) {
      throw FormatException('Invalid fields at rule $position');
    }
  }
  return rule;
}

Uint8List encodeAddedRules(Iterable<Rule> rules) {
  return Uint8List.fromList(
    utf8.encode(
      yaml.encode({'rules': rules.map((rule) => rule.rawValue).toList()}),
    ),
  );
}

Future<int> importAddedRules(List<Rule> rules) {
  return database.transaction(() async {
    final existing = await database.rulesDao.queryGlobalAddedRules().get();
    final seen = existing.map((rule) => rule.rawValue).toSet();
    var order = existing.lastOrNull?.order;
    var count = 0;
    for (final rule in rules) {
      if (!seen.add(rule.rawValue)) continue;
      order = indexing.generateKeyBetween(order, null);
      await database.rulesDao.putGlobalRule(
        rule.copyWith(id: snowflake.id, order: order),
      );
      count++;
    }
    return count;
  });
}
