Name:           ezpay
Version:        1.0.0
Release:        1%{?dist}
Summary:        A lightweight payment gateway supporting USDT, WeChat and Alipay

License:        MIT
URL:            https://github.com/yourusername/ezpay
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.21
BuildRequires:  systemd-rpm-macros
Requires:       glibc

%description
EzPay is a multi-chain cryptocurrency payment gateway supporting USDT
(TRC20/ERC20/BEP20/Polygon/Optimism/Arbitrum/Base), TRX, WeChat Pay and Alipay.
Features include automatic exchange rate updates, wallet rotation,
Telegram bot notifications and API compatibility with Rainbow Epay and VMQ.

%prep
%autosetup

%build
export CGO_ENABLED=0
export GOOS=linux
%ifarch x86_64
export GOARCH=amd64
%endif
%ifarch aarch64
export GOARCH=arm64
%endif
go build -ldflags="-s -w -X main.Version=%{version}" -o ezpay .

%install
# Install binary
install -Dm755 ezpay %{buildroot}%{_bindir}/ezpay

# Install config
install -Dm644 config.yaml %{buildroot}%{_sysconfdir}/ezpay/config.yaml

# Install web files
install -dm755 %{buildroot}%{_datadir}/ezpay/web
cp -r web/templates %{buildroot}%{_datadir}/ezpay/web/
cp -r web/static %{buildroot}%{_datadir}/ezpay/web/

# Install systemd service
install -Dm644 pkg/rpm/ezpay.service %{buildroot}%{_unitdir}/ezpay.service

# Create data directories
install -dm755 %{buildroot}%{_sharedstatedir}/ezpay
install -dm755 %{buildroot}%{_localstatedir}/log/ezpay

%pre
# Create ezpay user and group
getent group ezpay >/dev/null || groupadd -r ezpay
getent passwd ezpay >/dev/null || \
    useradd -r -g ezpay -d %{_sharedstatedir}/ezpay -s /sbin/nologin \
    -c "EzPay Payment Gateway" ezpay
exit 0

%post
%systemd_post ezpay.service
# Set ownership
chown -R ezpay:ezpay %{_sharedstatedir}/ezpay
chown -R ezpay:ezpay %{_localstatedir}/log/ezpay
chown -R ezpay:ezpay %{_datadir}/ezpay
chown ezpay:ezpay %{_sysconfdir}/ezpay/config.yaml
chmod 600 %{_sysconfdir}/ezpay/config.yaml

echo "============================================"
echo "EzPay installed successfully!"
echo ""
echo "Please edit /etc/ezpay/config.yaml before starting"
echo ""
echo "To start:  systemctl start ezpay"
echo "To enable: systemctl enable ezpay"
echo "============================================"

%preun
%systemd_preun ezpay.service

%postun
%systemd_postun_with_restart ezpay.service

%files
%license LICENSE
%doc README.md
%{_bindir}/ezpay
%dir %{_sysconfdir}/ezpay
%config(noreplace) %{_sysconfdir}/ezpay/config.yaml
%{_datadir}/ezpay
%{_unitdir}/ezpay.service
%dir %attr(755,ezpay,ezpay) %{_sharedstatedir}/ezpay
%dir %attr(755,ezpay,ezpay) %{_localstatedir}/log/ezpay

%changelog
* Thu Jan 02 2026 Your Name <your.email@example.com> - 1.0.0-1
- Initial release
- Multi-chain USDT support (TRC20/ERC20/BEP20/Polygon/Optimism/Arbitrum/Base)
- TRX native token support
- WeChat and Alipay integration
- Automatic exchange rate updates
- Wallet rotation for load balancing
- Telegram bot notifications
- Compatible with Rainbow Epay and VMQ APIs
