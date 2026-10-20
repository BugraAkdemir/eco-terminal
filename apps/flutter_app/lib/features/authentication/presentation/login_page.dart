import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/router/route_access.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/theme/tokens.dart';
import '../data/validators.dart';
import 'auth_scaffold.dart';

class LoginPage extends ConsumerStatefulWidget {
  const LoginPage({super.key, this.from, this.expired = false});

  /// Location to return to after login.
  final String? from;

  /// Redirected here because a 401 cleared the session.
  final bool expired;

  @override
  ConsumerState<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends ConsumerState<LoginPage> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  bool _busy = false;
  bool _obscure = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    if (widget.expired) {
      // The notice is shown from the query flag; clear the session flag.
      WidgetsBinding.instance.addPostFrameCallback(
        (_) => ref.read(sessionProvider.notifier).acknowledgeExpired(),
      );
    }
  }

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final s = context.s;
    if (!(_formKey.currentState?.validate() ?? false)) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(sessionProvider.notifier)
          .login(_email.text, _password.text);
      if (!mounted) return;
      context.go(widget.from ?? AppPaths.home);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = s.errorText(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    return AuthScaffold(
      title: s.login,
      notice: widget.expired
          ? Container(
              key: const ValueKey('session-expired'),
              padding: const EdgeInsets.all(Dimens.md),
              decoration: BoxDecoration(
                color: eco.warning.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(Dimens.radiusSm),
                border: Border.all(color: eco.warning.withValues(alpha: 0.6)),
              ),
              child: Text(s.sessionExpired),
            )
          : null,
      child: Form(
        key: _formKey,
        child: AutofillGroup(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (_error != null) FormErrorBanner(message: _error!),
              TextFormField(
                key: const ValueKey('login-email'),
                controller: _email,
                keyboardType: TextInputType.emailAddress,
                autofillHints: const [AutofillHints.email],
                textInputAction: TextInputAction.next,
                decoration: InputDecoration(labelText: s.email),
                validator: (v) => AuthValidators.email(s, v),
              ),
              const SizedBox(height: Dimens.md),
              TextFormField(
                key: const ValueKey('login-password'),
                controller: _password,
                obscureText: _obscure,
                autofillHints: const [AutofillHints.password],
                textInputAction: TextInputAction.done,
                onFieldSubmitted: (_) => _submit(),
                decoration: InputDecoration(
                  labelText: s.password,
                  suffixIcon: IconButton(
                    tooltip: s.password,
                    icon: Icon(_obscure ? Icons.visibility : Icons.visibility_off),
                    onPressed: () => setState(() => _obscure = !_obscure),
                  ),
                ),
                validator: (v) => AuthValidators.passwordPresent(s, v),
              ),
              const SizedBox(height: Dimens.xl),
              FilledButton(
                key: const ValueKey('login-submit'),
                onPressed: _busy ? null : _submit,
                child: _busy
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(s.login),
              ),
              const SizedBox(height: Dimens.md),
              Wrap(
                alignment: WrapAlignment.center,
                crossAxisAlignment: WrapCrossAlignment.center,
                children: [
                  Text(s.noAccountYet),
                  TextButton(
                    key: const ValueKey('go-register'),
                    onPressed: () => context.go(
                      Uri(
                        path: AppPaths.register,
                        queryParameters:
                            widget.from == null ? null : {'from': widget.from},
                      ).toString(),
                    ),
                    child: Text(s.register),
                  ),
                ],
              ),
              TextButton(
                key: const ValueKey('continue-guest'),
                onPressed: () => context.go(AppPaths.home),
                child: Text(s.continueAsGuest),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
