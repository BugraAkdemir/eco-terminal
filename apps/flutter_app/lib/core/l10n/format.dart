import 'package:flutter/widgets.dart';
import 'package:intl/date_symbol_data_local.dart';
import 'package:intl/intl.dart';

/// Loads date symbols for the supported locales (call once at startup).
Future<void> initFormatting() async {
  await initializeDateFormatting('tr');
  await initializeDateFormatting('en');
}

String _locale(BuildContext context) =>
    Localizations.localeOf(context).languageCode;

/// `20 Eki 2026` / `Oct 20, 2026`. Falls back to ISO date when locale data is
/// not loaded (tests).
String formatDate(BuildContext context, DateTime? d) {
  if (d == null) return '';
  final local = d.toLocal();
  try {
    return DateFormat.yMMMd(_locale(context)).format(local);
  } catch (_) {
    return local.toIso8601String().substring(0, 10);
  }
}

String formatDateTime(BuildContext context, DateTime? d) {
  if (d == null) return '';
  final local = d.toLocal();
  try {
    return DateFormat.yMMMd(_locale(context)).add_Hm().format(local);
  } catch (_) {
    return local.toIso8601String().substring(0, 16).replaceFirst('T', ' ');
  }
}

/// `YYYY-MM-DD` for API params / labels.
String isoDay(DateTime d) =>
    '${d.year.toString().padLeft(4, '0')}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';

String formatNumber(BuildContext context, num n) {
  try {
    return NumberFormat.decimalPattern(_locale(context)).format(n);
  } catch (_) {
    return n.toString();
  }
}
