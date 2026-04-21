package mfix

import (
	"testing"
)

func TestChecksumAndBodyLength(t *testing.T) {
	// 8=FIX.4.2|9=49|35=0|49=SENDER|56=TARGET|34=1|52=20230101-12:00:00|10=123|
	msg := Message{
		{8, "FIX.4.2"},
		{9, "49"},
		{35, "0"},
		{49, "SENDER"},
		{56, "TARGET"},
		{34, "1"},
		{52, "20230101-12:00:00"},
		{10, "123"},
	}

	t.Run("BodyLength Calculation", func(t *testing.T) {
		got := BodyLength(&msg)
		expected := uint64(51)
		if got != expected {
			t.Errorf("BodyLength() = %d; want %d", got, expected)
		}
	})

	t.Run("Checksum Calculation", func(t *testing.T) {
		got := Checksum(&msg)
		if got == 0 {
			t.Error("Checksum returned 0, likely failure in calculation logic")
		}
	})

	t.Run("Position Agnostic Check", func(t *testing.T) {
		// Swap 49 and 56
		msgSwapped := Message{
			{8, "FIX.4.2"},
			{9, "49"},
			{35, "0"},
			{56, "TARGET"},
			{49, "SENDER"},
			{34, "1"},
			{52, "20230101-12:00:00"},
		}

		if Checksum(&msg) != Checksum(&msgSwapped) {
			t.Error("Checksum changed after field swap; parser is not position-agnostic")
		}
		if BodyLength(&msg) != BodyLength(&msgSwapped) {
			t.Error("BodyLength changed after field swap")
		}
	})
}

func TestChecksumVerification(t *testing.T) {
	tests := []struct {
		name     string
		msg      Message
		expLen   uint64
		expCheck uint8
	}{
		{
			name: "Logon FIX 4.4",
			msg: Message{
				{8, "FIX.4.4"},
				{9, "75"},
				{35, "A"},
				{34, "1092"},
				{49, "TESTBUY1"},
				{52, "20180920-18:24:59.643"},
				{56, "TESTSELL1"},
				{98, "0"},
				{108, "60"},
			},
			expLen:   75,
			expCheck: 178,
		},
		{
			name: "Logout FIX 4.4",
			msg: Message{
				{8, "FIX.4.4"},
				{9, "63"},
				{35, "5"},
				{34, "1091"},
				{49, "TESTBUY1"},
				{52, "20180920-18:24:58.675"},
				{56, "TESTSELL1"},
			},
			expLen:   63,
			expCheck: 138,
		},
		{
			name: "Allocation FIX 4.2",
			msg: Message{
				{8, "FIX.4.2"},
				{9, "127"},
				{35, "P"},
				{34, "936"},
				{49, "TESTSELL3"},
				{52, "20260324-15:45:13.992"},
				{56, "TESTBUY3"},
				{60, "20260324-15:45:13.992"},
				{70, "3639096067028819307"},
				{75, "20230625"},
				{87, "0"},
			},
			expLen:   127,
			expCheck: 41,
		},
		{
			name: "NewOrderSingle FIX 4.2",
			msg: Message{
				{8, "FIX.4.2"},
				{9, "163"},
				{35, "D"},
				{34, "972"},
				{49, "TESTBUY3"},
				{52, "20190206-16:25:10.403"},
				{56, "TESTSELL3"},
				{11, "141636850670842269979"},
				{21, "2"},
				{38, "100"},
				{40, "1"},
				{54, "1"},
				{55, "AAPL"},
				{60, "20190206-16:25:08.968"},
				{207, "TO"},
				{6000, "TEST1234"},
			},
			expLen:   163,
			expCheck: 106,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualLen := BodyLength(&tt.msg)
			if actualLen != tt.expLen {
				t.Errorf("%s: BodyLength = %d, want %d", tt.name, actualLen, tt.expLen)
			}

			actualCheck := Checksum(&tt.msg)
			if actualCheck != tt.expCheck {
				t.Errorf("%s: Checksum = %d, want %d", tt.name, actualCheck, tt.expCheck)
			}
		})
	}
}
