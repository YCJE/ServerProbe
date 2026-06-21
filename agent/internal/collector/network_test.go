package collector

import (
	"testing"

	"github.com/server-probe/shared/model"
)

const netDevSample1 = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 1048576000 1234567    0    0    0     0          0         0 524288000 987654    0    0    0     0       0          0
    lo: 12345678  12345    0    0    0     0          0         0 12345678  12345    0    0    0     0       0          0
`

const netDevSample2 = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 1073741824 1235567    0    0    0     0          0         0 536870912 989654    0    0    0     0       0          0
    lo: 12345678  12345    0    0    0     0          0         0 12345678  12345    0    0    0     0       0          0
`

const tcpSample = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 0100007F:D4 01 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 20 4 30 10 -1
   2: 0100007F:D4 0100007F:1F90 01 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 20 4 30 10 -1
   3: 0202A8C0:831 6C2E0A63:01BB 01 00000000:00000000 00:00000000 00000000     0        0 12348 1 0000000000000000 20 4 30 10 -1
`

const udpSample = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:1F90 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0
`

func TestNetworkCollector_Collect(t *testing.T) {
	reader := &mockFileReader{
		files: map[string][]byte{
			"/proc/net/dev": []byte(netDevSample1),
			"/proc/net/tcp": []byte(tcpSample),
			"/proc/net/udp": []byte(udpSample),
		},
	}

	collector := NewNetworkCollector(reader)

	// 第一次采集
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("第一次采集失败: %v", err)
	}

	netInfo, ok := result.(model.NetworkInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 model.NetworkInfo，得到 %T", result)
	}

	// 第一次采集速率为 0
	if netInfo.RxSpeed != 0 {
		t.Errorf("首次采集下行速率应为 0, 得到 %d", netInfo.RxSpeed)
	}
	if netInfo.TxSpeed != 0 {
		t.Errorf("首次采集上行速率应为 0, 得到 %d", netInfo.TxSpeed)
	}

	// TCP 连接数：4 条
	if netInfo.TCPConnections != 4 {
		t.Errorf("TCP 连接数错误: 期望 4, 得到 %d", netInfo.TCPConnections)
	}

	// UDP 连接数：2 条
	if netInfo.UDPConnections != 2 {
		t.Errorf("UDP 连接数错误: 期望 2, 得到 %d", netInfo.UDPConnections)
	}
}

func TestNetworkCollector_SpeedCalculation(t *testing.T) {
	reader := &mockFileReader{
		files: map[string][]byte{
			"/proc/net/dev": []byte(netDevSample1),
			"/proc/net/tcp": []byte(tcpSample),
			"/proc/net/udp": []byte(udpSample),
		},
	}

	collector := NewNetworkCollector(reader)

	// 第一次采集
	_, err := collector.Collect()
	if err != nil {
		t.Fatalf("第一次采集失败: %v", err)
	}

	// 更新 mock 数据为第二次采样
	reader.files["/proc/net/dev"] = []byte(netDevSample2)

	// 模拟 3 秒间隔
	collector.SetElapsed(3)

	// 第二次采集
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("第二次采集失败: %v", err)
	}

	netInfo, ok := result.(model.NetworkInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 model.NetworkInfo，得到 %T", result)
	}

	// eth0 RX: 1073741824 - 1048576000 = 25165824 bytes
	// 3 秒间隔，速率 = 25165824 / 3 = 8388608 bytes/s
	expectedRxSpeed := uint64(25165824 / 3)
	if netInfo.RxSpeed != expectedRxSpeed {
		t.Errorf("下行速率错误: 期望 %d, 得到 %d", expectedRxSpeed, netInfo.RxSpeed)
	}

	// eth0 TX: 536870912 - 524288000 = 12582912 bytes
	// 3 秒间隔，速率 = 12582912 / 3 = 4194304 bytes/s
	expectedTxSpeed := uint64(12582912 / 3)
	if netInfo.TxSpeed != expectedTxSpeed {
		t.Errorf("上行速率错误: 期望 %d, 得到 %d", expectedTxSpeed, netInfo.TxSpeed)
	}
}

func TestNetworkCollector_Name(t *testing.T) {
	collector := NewNetworkCollector(&OSFileReader{})
	if collector.Name() != "network" {
		t.Errorf("采集器名称错误: 期望 network, 得到 %s", collector.Name())
	}
}

func TestParseNetDev(t *testing.T) {
	rx, tx, err := parseNetDev(netDevSample1)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// eth0 RX: 1048576000（lo 被排除）
	expectedRx := uint64(1048576000)
	if rx != expectedRx {
		t.Errorf("RX 错误: 期望 %d, 得到 %d", expectedRx, rx)
	}

	// eth0 TX: 524288000（lo 被排除）
	expectedTx := uint64(524288000)
	if tx != expectedTx {
		t.Errorf("TX 错误: 期望 %d, 得到 %d", expectedTx, tx)
	}
}

func TestCountConnections(t *testing.T) {
	tcpCount := countConnections(tcpSample)
	if tcpCount != 4 {
		t.Errorf("TCP 连接数错误: 期望 4, 得到 %d", tcpCount)
	}

	udpCount := countConnections(udpSample)
	if udpCount != 2 {
		t.Errorf("UDP 连接数错误: 期望 2, 得到 %d", udpCount)
	}
}
