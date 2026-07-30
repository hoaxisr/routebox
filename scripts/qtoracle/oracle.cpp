// Reads a full "vpn://..." link on stdin, decodes it exactly as the Amnezia client
// does, runs it through the client's own AwgProtocolConfig parser, and prints the
// result as JSON. Whatever comes out is what the app would hand to its VPN daemon.
#include <QCoreApplication>
#include <QByteArray>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QString>
#include <QTextStream>

#include "core/models/protocols/awgProtocolConfig.h"

using namespace amnezia;

int main(int argc, char **argv) {
    QCoreApplication app(argc, argv);

    QFile in;
    in.open(stdin, QIODevice::ReadOnly);
    QString link = QString::fromUtf8(in.readAll()).trimmed();

    // importController.cpp:166-175
    link.replace("vpn://", "");
    QByteArray ba = QByteArray::fromBase64(
        link.toUtf8(), QByteArray::Base64UrlEncoding | QByteArray::OmitTrailingEquals);
    QByteArray un = qUncompress(ba);
    if (!un.isEmpty()) {
        ba = un;
    }

    QJsonDocument doc = QJsonDocument::fromJson(ba);
    if (!doc.isObject()) {
        QTextStream(stderr) << "payload did not decode to a JSON object\n";
        return 2;
    }
    QJsonArray containers = doc.object().value("containers").toArray();
    if (containers.isEmpty()) {
        QTextStream(stderr) << "no containers\n";
        return 2;
    }
    QJsonObject c = containers.at(0).toObject();
    // This ternary MIRRORS the generator's own container-name decision (awg vs
    // wireguard) rather than independently verifying it — it just needs to pick the
    // same protocol key the generator used so fromJson() has an object to parse.
    // Consequently the oracle does NOT check defaultContainer, description, dns1/dns2,
    // or that this branch chose the right container for the peer's obfuscation state;
    // those stay covered by the untagged Go tests in vpnlink_test.go.
    QString proto = c.value("container").toString() == "amnezia-awg" ? "awg" : "wireguard";

    AwgProtocolConfig cfg = AwgProtocolConfig::fromJson(c.value(proto).toObject());
    if (!cfg.hasClientConfig()) {
        QTextStream(stderr) << "no client config parsed from last_config\n";
        return 3;
    }
    // toJson() emits the server fields AND last_config, so both parsers are exercised.
    // Note for whoever reads the printed outer object: AwgServerConfig::toJson()
    // unconditionally emits I1-I5 (specialJunk1-5), even when empty, unlike
    // AwgClientConfig::toJson() which guards each one with isEmpty(). A Mimic-less run
    // therefore shows "I1":"" etc. in the OUTER object — that is the client's own
    // asymmetry, not a bug in this oracle or in our payload.
    QTextStream(stdout) << QJsonDocument(cfg.toJson()).toJson(QJsonDocument::Compact);
    return 0;
}
