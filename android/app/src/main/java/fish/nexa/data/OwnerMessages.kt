package fish.nexa.data

import fish.nexa.Nexa
import fish.nexa.crypto.*
import fish.nexa.net.Message
import fish.nexa.ui.legal.Legal
import kotlinx.serialization.json.*

object OwnerMessages {
    suspend fun attach(env: Envelope, owner: String?, author: String, channel: String, payload: MessagePayload, approvingAccount: String = author): JsonObject {
        require(!owner.isNullOrBlank() && owner != "0" && Legal.accepted(approvingAccount))
        val key = requireNotNull(Nexa.keys.sealingKeyOf(owner)) { "Owner identity key changed" }
        val data = buildJsonObject {
            put("author", author)
            put("channel", channel)
            put("iv", env.iv)
            put("payload", payload.toJson())
        }
        val box = Bytes.b64(fish.nexa.crypto.Keys.seal(key, Bytes.utf8(data.toString())))
        return JsonObject(env.toJson() + ("owner" to buildJsonObject { put("id", owner); put("box", box) }))
    }

    fun read(m: Message): MessagePayload {
        if (m.webhook != null && (m.ciphertext == null || m.ciphertext is JsonNull)) {
            return requireNotNull(Nexa.messages.decrypt(m).payload)
        }
        val identity = requireNotNull(Nexa.keys.identity)
        val env = requireNotNull(m.ciphertext).jsonObject
        val owner = env.getValue("owner").jsonObject
        require(owner.getValue("id").jsonPrimitive.content == m.ownerRecipient)
        val bytes = fish.nexa.crypto.Keys.open(identity.privateKey, identity.publicKey, Bytes.unb64(owner.getValue("box").jsonPrimitive.content))
        val row = MessageCrypto.json.parseToJsonElement(Bytes.str(bytes)).jsonObject
        require(row["author"]?.jsonPrimitive?.content == m.authorId && row["channel"]?.jsonPrimitive?.content == m.channelId && row["iv"] == env["iv"])
        return MessageCrypto.sanitizePayload(row.getValue("payload").jsonObject)
    }
}
