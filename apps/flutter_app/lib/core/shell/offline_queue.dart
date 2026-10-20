import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Hook for a feature module that stores unsent records on the device (the
/// observation wizard). The foundation only displays it: the offline banner
/// promises local storage only when [supported] is true, and the Profil tab
/// shows a badge with [pending].
class OfflineQueueState {
  const OfflineQueueState({this.supported = false, this.pending = 0});
  final bool supported;
  final int pending;
}

class OfflineQueueController extends Notifier<OfflineQueueState> {
  @override
  OfflineQueueState build() => const OfflineQueueState();

  void setSupported(bool v) =>
      state = OfflineQueueState(supported: v, pending: state.pending);

  void setPending(int n) =>
      state = OfflineQueueState(supported: state.supported, pending: n);
}

final offlineQueueProvider =
    NotifierProvider<OfflineQueueController, OfflineQueueState>(
  OfflineQueueController.new,
);
