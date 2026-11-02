import 'package:fl_clash/common/constant.dart';
import 'package:fl_clash/models/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('legacy settings retain Sticky defaults', () {
    final settings = AppSettingProps.fromJson({});
    expect(settings.stickyTestUrl, defaultStickyTestUrl);
    expect(settings.stickyTimeout, 5);
    expect(settings.stickyFailureThreshold, 2);
  });

  test('Sticky settings persist independently from manual delay tests', () {
    const settings = AppSettingProps(
      testUrl: 'https://manual.example/204',
      stickyFailureThreshold: 7,
      stickyTestUrl: 'https://health.example/204',
      stickyTimeout: 12,
    );
    expect(AppSettingProps.fromJson(settings.toJson()), settings);
  });

  test('invalid restored values only reset the affected Sticky fields', () {
    for (final url in [
      '',
      'ftp://example.com',
      'https://',
      'https://u:p@example.com',
      42,
    ]) {
      for (final timeout in [0, -1, 61, '5', 1.5]) {
        final settings = AppSettingProps.fromJson({
          'autoLaunch': true,
          'stickyTestUrl': url,
          'stickyTimeout': timeout,
        });
        expect(settings.autoLaunch, true);
        expect(settings.stickyTestUrl, defaultStickyTestUrl);
        expect(settings.stickyTimeout, defaultStickyTimeout);
      }
    }
    for (final seconds in [1, 60]) {
      final settings = AppSettingProps.fromJson({
        'stickyTestUrl': ' http://localhost:8080/check ',
        'stickyTimeout': seconds,
      });
      expect(settings.stickyTestUrl, 'http://localhost:8080/check');
      expect(settings.stickyTimeout, seconds);
    }
  });

  test('failure threshold restores only integers from 1 to 10', () {
    for (final value in [0, -1, 11, '2', 1.5]) {
      final settings = AppSettingProps.fromJson({
        'stickyFailureThreshold': value,
      });
      expect(settings.stickyFailureThreshold, 2);
    }
    for (var value = 1; value <= 10; value++) {
      final settings = AppSettingProps.fromJson({
        'stickyFailureThreshold': value,
      });
      expect(settings.stickyFailureThreshold, value);
      expect(AppSettingProps.fromJson(settings.toJson()), settings);
    }
  });

  test('setup contract uses kebab-case keys and legacy defaults', () {
    const params = SetupParams(
      selectedMap: {},
      testUrl: defaultTestUrl,
      stickyFailureThreshold: 7,
      stickyTestUrl: 'https://health.example/204',
      stickyTimeout: 12,
    );
    expect(
      params.toJson(),
      containsPair('sticky-test-url', params.stickyTestUrl),
    );
    expect(params.toJson(), containsPair('sticky-timeout', 12));
    expect(params.toJson(), containsPair('sticky-failure-threshold', 7));
    expect(SetupParams.fromJson(params.toJson()), params);
    final legacy = SetupParams.fromJson({
      'selected-map': {},
      'test-url': defaultTestUrl,
    });
    expect(legacy.stickyTestUrl, defaultStickyTestUrl);
    expect(legacy.stickyTimeout, 5);
  });
}
