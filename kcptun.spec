%global         debug_package   %{nil}
%global         _dwz_low_mem_die_limit 0
%global         __os_install_post /usr/lib/rpm/brp-compress %{nil}

Name:           kcptun
Version:        20210922
Release:        1
Summary:        A Stable & Secure Tunnel based on KCP with N:M multiplexing and FEC

License:        MIT
URL:            https://github.com/xtaci/kcptun
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang
BuildRequires:  glibc-static

%description
A Stable & Secure Tunnel based on KCP with N:M multiplexing and FEC

%package	server
Summary:	kcptun-server
Requires:       systemd

%description	server
A Stable & Secure Tunnel based on KCP with N:M multiplexing and FEC

%package	client
Summary:	kcptun-client
Requires:       systemd

%description	client
A Stable & Secure Tunnel based on KCP with N:M multiplexing and FEC

%prep
%autosetup

%build
LDFLAGS='-s -w -linkmode=external -extldflags -static -X main.VERSION=%{version}'
go build -ldflags "$LDFLAGS" -o %{name}-server github.com/xtaci/kcptun/server
go build -ldflags "$LDFLAGS" -o %{name}-client github.com/xtaci/kcptun/client

%install
rm -rf $RPM_BUILD_ROOT

%{__mkdir} -p $RPM_BUILD_ROOT%{_bindir}
%{__install} -p -m 755 kcptun-server $RPM_BUILD_ROOT%{_bindir}/kcptun-server
%{__install} -p -m 755 kcptun-client $RPM_BUILD_ROOT%{_bindir}/kcptun-client

%{__mkdir} -p $RPM_BUILD_ROOT%{_sysconfdir}/%{name}
%{__install} -p -m 644 examples/server.json $RPM_BUILD_ROOT%{_sysconfdir}/%{name}/server.json
%{__install} -p -m 644 examples/local.json $RPM_BUILD_ROOT%{_sysconfdir}/%{name}/client.json

%{__mkdir} -p $RPM_BUILD_ROOT%{_unitdir}
cat > $RPM_BUILD_ROOT%{_unitdir}/kcptun-server.service <<EOF
[Unit]
Description=kcptun-server
Wants=network.target
After=syslog.target network-online.target
ConditionFileIsExecutable=/usr/bin/kcptun-server
ConditionPathExists=/etc/kcptun/server.json

[Service]
Type=simple
Environment=GOGC=20
ExecStart=/usr/bin/kcptun-server -c /etc/kcptun/server.json
Restart=on-failure
RestartSec=10
KillMode=process
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

cat > $RPM_BUILD_ROOT%{_unitdir}/kcptun-client.service <<EOF
[Unit]
Description=kcptun-client
After=syslog.target network-online.target
ConditionFileIsExecutable=/usr/bin/kcptun-client
ConditionPathExists=/etc/kcptun/client.json

[Service]
Type=simple
Environment=GOGC=20
ExecStart=/usr/bin/kcptun-client -c /etc/kcptun/client.json
Restart=on-failure
RestartSec=10
KillMode=process
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

%post server
%systemd_post %{name}-server.service

%preun server
%systemd_preun %{name}-server.service

%postun server
%systemd_postun_with_restart %{name}-server.service

%post client
%systemd_post %{name}-client.service

%preun client
%systemd_preun %{name}-client.service

%postun client
%systemd_postun_with_restart %{name}-client.service

%files server
%defattr(-,root,root,-)
%{_bindir}/kcptun-server
%config(noreplace) %{_sysconfdir}/%{name}/server.json
%{_unitdir}/kcptun-server.service
%license LICENSE.md
%doc README.md Dockerfile

%files client
%defattr(-,root,root,-)
%{_bindir}/kcptun-client
%config(noreplace) %{_sysconfdir}/%{name}/client.json
%{_unitdir}/kcptun-client.service
%license LICENSE.md
%doc README.md Dockerfile

%changelog
* Thu Dec 30 2021 Purple Grape <purplegrape4@gmail.com>
- First package for kcptun
