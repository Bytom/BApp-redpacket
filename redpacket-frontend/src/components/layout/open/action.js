import {
  openRedPacket
} from '../../util/api'
import {getCurrentAddress} from "../../util/utils";

export function open(value,redPackId) {
  const address = getCurrentAddress()
  const password = value.password.trim()

  return openRedPacket({
    "red_packet_id": redPackId,
    "address": address,
    "password": password
  })
}
