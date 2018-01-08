// Copyright (C) 2017, Beijing Bochen Technology Co.,Ltd.  All rights reserved.
//
// This file is part of L0
//
// The L0 is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The L0 is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package luavm

import (
	//"context"
	"errors"
	"strings"
	//"time"

	"github.com/bocheninc/L0/components/log"
	"github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
	"github.com/bocheninc/L0/nvm"
	"strconv"
	"github.com/bocheninc/L0/core/params"
	"fmt"
	"encoding/json"
	//"sync"
	"math/rand"
)

//var vmproc *vm.VMProc
//var luaProto = make(map[string]*lua.FunctionProto)

// Start start vm process
type LuaWorker struct {
	isInit bool
	workerFlag int
	luaProto  map[string]*lua.FunctionProto
	luaLFunc  map[string]*lua.LFunction
	L *lua.LState
	VMConf *nvm.Config
	workerProc *nvm.WorkerProc
}

func NewLuaWorker(conf *nvm.Config) *LuaWorker {
	worker := &LuaWorker{isInit: false}
	worker.workerInit(true, conf)

	return worker
}

func (worker *LuaWorker) VmJob(data interface{}) interface{} {
	//startTime := time.Now()
	workerProcWithCallback := data.(*nvm.WorkerProcWithCallback)
	_, err := worker.requestHandle(workerProcWithCallback.WorkProc)
	if err != nil {
		log.Errorf("err: %+v", err)
	} else {
		workerProcWithCallback.Fn("hello")
	}

	//execTime := time.Now().Sub(startTime)
	//log.Debugf("===> [%d]must execTime0: %s", worker.workerFlag, execTime)
	//result, err := worker.requestHandle(data.(*nvm.WorkerProc))
	//log.Debugf("requestResult, result: %+v, err: %+v", result, err)
	return nil
}

func (worker *LuaWorker) VmReady() bool {
	return true
}

func (worker *LuaWorker) VmInitialize() {
	if !worker.isInit {
		worker.workerInit(true, nvm.DefaultConfig())
	}
}

func (worker *LuaWorker) VmTerminate() {
	//pass
	worker.L.Close()
}

func (worker *LuaWorker)requestHandle(wp *nvm.WorkerProc) (interface{}, error) {
	//log.Debug("call luavm FuncName:", wp.PreMethod)
	switch wp.PreMethod {
	case "PreInitContract":
		return worker.InitContract(wp)
	case "RealInitContract":
		return worker.InitContract(wp)
	case "PreInvokeExecute":
		return worker.InvokeExecute(wp)
	case "RealInvokeExecute":
		return worker.InvokeExecute(wp)
	case "QueryContract":
		return worker.QueryContract(wp)
	}
	return nil, errors.New("luavm no method match:" + wp.PreMethod)
}


func (worker *LuaWorker) InitContract(wp *nvm.WorkerProc) (interface{}, error) {
	worker.resetProc(wp)
	worker.StoreContractCode()
	ok, err := worker.execContract(wp.ContractData, "L0Init")
	if err != nil {
		return false, err
	}

	if _, ok := ok.(bool); !ok {
		return false, errors.New("InitContract execContract result type is not bool")
	}

	err = worker.workerProc.CCallCommit()

	if err != nil {
		log.Errorf("commit all change error contractAddr:%s, errmsg:%s\n", worker.workerProc.ContractData.ContractAddr, err.Error())
		worker.workerProc.CCallSmartContractFailed()
		return false, err
	}

	return ok, err
}

func (worker *LuaWorker) InvokeExecute(wp *nvm.WorkerProc) (interface{}, error) {
	worker.resetProc(wp)
	//TODO to code >>> wp.ContractData
	code, err := worker.GetContractCode()
	if err != nil {
		log.Errorf("can't get contract code")
	}

	wp.ContractData.ContractCode = string(code)
	ok, err := worker.execContract(wp.ContractData, "L0Invoke")
	if err != nil {
		return false, err
	}

	if _, ok := ok.(bool); !ok {
		return false, errors.New("RealExecute execContract result type is not bool")
	}

	err = worker.workerProc.CCallCommit()

	if err != nil {
		log.Errorf("commit all change error contractAddr:%s, errmsg:%s\n", worker.workerProc.ContractData.ContractAddr, err.Error())
		worker.workerProc.CCallSmartContractFailed()
		return false, err
	}

	return ok, err
}

func (worker *LuaWorker)QueryContract(wp *nvm.WorkerProc) ([]byte, error) {
	worker.resetProc(wp)
	value, err := worker.execContract(wp.ContractData, "L0Query")
	if err != nil {
		return nil, err
	}

	result, ok := value.(string)
	if !ok {
		return nil, errors.New("QueryContract execContract result type is not string")
	}

	return []byte(result), nil
}

func (worker *LuaWorker) resetProc(wp *nvm.WorkerProc) {
	worker.workerProc = wp
	//startTime := time.Now()
	//loader := func(L *lua.LState) int {
	//	mod := L.SetFuncs(L.NewTable(), exporter(worker.workerProc)) // register functions to the table
	//	L.Push(mod)
	//	return 1
	//}
	////execTime := time.Now().Sub(startTime)
	////log.Debugf("exec time: %s", execTime)
	//worker.L.PreloadModule("L0", loader)
	wp.StateChangeQueue = nvm.NewStateQueue()
	wp.TransferQueue = nvm.NewTransferQueue()
}


func (worker *LuaWorker) workerInit(isInit bool, vmconf *nvm.Config) {
	worker.isInit = true
	worker.VMConf = vmconf
	worker.luaProto = make(map[string]*lua.FunctionProto)
	worker.luaLFunc = make(map[string]*lua.LFunction)
	worker.workerFlag = rand.Int()
	worker.workerProc = &nvm.WorkerProc{}
	worker.L = worker.newState()
	loader := func(L *lua.LState) int {
		mod := L.SetFuncs(L.NewTable(), exporter(worker.workerProc)) // register functions to the table
		L.Push(mod)
		return 1
	}
	worker.L.PreloadModule("L0", loader)
}

// execContract start a lua vm and execute smart contract script
func (worker *LuaWorker) execContract(cd *nvm.ContractData, funcName string) (interface{}, error) {
	//log.Debugf("luaVM execContract funcName:%s, contractAddr: %+v, contractParams: %+v", funcName, cd.ContractAddr, cd.ContractParams)
	//var gog sync.WaitGroup
	//defer func() {
	//	gog.Wait()
	//}()
	defer func() {
		if e := recover(); e != nil {
			log.Error("LuaVM exec contract code error ", e)
		}
	}()
	//
	if err := worker.CheckContractCode(cd.ContractCode); err != nil {
		return false, err
	}

	//worker.L = worker.newState()
	//defer worker.L.Close()
	//
	////ctx, cancel := context.WithTimeout(context.Background(), time.Duration(worker.VMConf.ExecLimitMaxRunTime)*time.Millisecond)
	////defer cancel()
	////
	////worker.L.SetContext(ctx)
	//
	//ctx, cancel := context.WithCancel(context.Background())
	//worker.L.SetContext(ctx)
	//timeOut := time.Duration(worker.VMConf.ExecLimitMaxRunTime) * time.Millisecond
	//timeOutChann := make(chan bool, 1)
	//defer func() {
	//	timeOutChann <- true
	//}()
	//
	//go func() {
	//	gog.Add(1)
	//	defer gog.Done()
	//	select {
	//	case <-timeOutChann:
	//		worker.L.RemoveContext()
	//	case <-time.After(timeOut):
	//		cancel()
	//	}
	//}()

	//startTime := time.Now()
	//loader := func(L *lua.LState) int {
	//	mod := L.SetFuncs(L.NewTable(), exporter(worker.workerProc)) // register functions to the table
	//	L.Push(mod)
	//	return 1
	//}
	//worker.L.PreloadModule("L0", loader)


	_, ok := worker.luaProto[cd.ContractAddr]
	if !ok {
		chunk, err := parse.Parse(strings.NewReader(cd.ContractCode), "<string>")
		if err != nil {
			return nil, err
		}
		proto, err := lua.Compile(chunk, "<string>")
		if err != nil {
			return nil, err
		}
		worker.luaProto[cd.ContractAddr] = proto
	}

	_, ok = worker.luaLFunc[cd.ContractAddr]
	if !ok {
		fn := &lua.LFunction{
			IsG: false,
			Env: worker.L.Env,

			Proto:     worker.luaProto[cd.ContractAddr],
			GFunction: nil,
			Upvalues:  make([]*lua.Upvalue, 0)}
		worker.L.Push(fn)
	}

	//fn := &lua.LFunction{
	//	IsG: false,
	//	Env: worker.L.Env,
	//
	//	Proto:     worker.luaProto[cd.ContractAddr],
	//	GFunction: nil,
	//	Upvalues:  make([]*lua.Upvalue, 0)}
	//worker.L.Push(fn)
	//

	if err := worker.L.PCall(0, lua.MultRet, nil); err != nil {
		return false, err
	}

	callLuaFuncResult, err := worker.callLuaFunc(worker.L, funcName, cd.ContractParams...)

	return callLuaFuncResult, err
	//log.Debugf("luaVM execContract funcName:%s\n", funcName)
	//=========================================================================
	//defer func() {
	//	if e := recover(); e != nil {
	//		log.Error("LuaVM exec contract code error ", e)
	//	}
	//}()
	//
	//code := cd.ContractCode
	//if err := worker.CheckContractCode(code); err != nil {
	//	return false, err
	//}

	//L := worker.newState()
	//defer L.Close()

	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(worker.VMConf.ExecLimitMaxRunTime)*time.Millisecond)
	//defer cancel()
	//
	//worker.L.SetContext(ctx)
	//
	//loader := func(L *lua.LState) int {
	//	mod := L.SetFuncs(L.NewTable(), exporter(worker.workerProc)) // register functions to the table
	//	L.Push(mod)
	//	return 1
	//}
	//worker.L.PreloadModule("L0", loader)

	//_, ok := worker.luaProto[cd.ContractAddr]
	//if !ok {
	//	chunk, err := parse.Parse(strings.NewReader(cd.ContractCode), "<string>")
	//	if err != nil {
	//		return nil, err
	//	}
	//	proto, err := lua.Compile(chunk, "<string>")
	//	if err != nil {
	//		return nil, err
	//	}
	//	worker.luaProto[cd.ContractAddr] = proto
	//}
	//
	//fn := &lua.LFunction{
	//	IsG: false,
	//	Env: worker.L.Env,
	//
	//	Proto:     worker.luaProto[cd.ContractAddr],
	//	GFunction: nil,
	//	Upvalues:  make([]*lua.Upvalue, 0)}
	//
	//worker.L.Push(fn)
	//
	//if err := worker.L.PCall(0, lua.MultRet, nil); err != nil {
	//	return false, err
	//}
	//
	//return worker.callLuaFunc(worker.L, funcName, cd.ContractParams...)
}

func (worker *LuaWorker) GetContractCode() (string, error) {
	var err error
	cc := new(nvm.ContractCode)
	var code []byte
	if len(worker.workerProc.ContractData.ContractAddr) == 0 {
		code, err = worker.workerProc.L0Handler.GetGlobalState(params.GlobalContractKey)
	} else {
		code, err = worker.workerProc.L0Handler.GetState(nvm.ContractCodeKey)
	}

	if len(code) != 0 && err == nil {
		contractCode, err := nvm.DoContractStateData(code)
		if err != nil {
			return "", fmt.Errorf("cat't find contract code in db, err: %+v", err)
		}
		err = json.Unmarshal(contractCode, cc)
		if err != nil {
			return "", fmt.Errorf("cat't find contract code in db, err: %+v", err)
		}

		return string(cc.Code), nil
	} else {
		return "", errors.New("cat't find contract code in db")
	}
}

func (worker *LuaWorker) StoreContractCode() error {
	code, err := nvm.ConcrateStateJson(&nvm.ContractCode{Code: []byte(worker.workerProc.ContractData.ContractCode), Type: "luavm"})
	if err != nil {
		log.Errorf("Can't concrate contract code")
	}

	if len(worker.workerProc.ContractData.ContractAddr) == 0 {
		err = worker.workerProc.CCallPutState(params.GlobalContractKey, code.Bytes())
	} else {
		err = worker.workerProc.CCallPutState(nvm.ContractCodeKey, code.Bytes()) // add js contract code into state
	}

	return  err
}

func (worker *LuaWorker)CheckContractCode(code string) error {
	if len(code) == 0 || len(code) > worker.VMConf.ExecLimitMaxScriptSize {
		return errors.New("contract script code size illegal " +
			strconv.Itoa(len(code)) +
			"byte , max size is:" +
			strconv.Itoa(worker.VMConf.ExecLimitMaxScriptSize) + " byte")
	}

	return nil
}

// newState create a lua vm
func (worker *LuaWorker) newState() *lua.LState {
	opt := lua.Options{
		SkipOpenLibs:        true,
		CallStackSize:       worker.VMConf.VMCallStackSize,
		RegistrySize:        worker.VMConf.VMRegistrySize,
		MaxAllowOpCodeCount: worker.VMConf.ExecLimitMaxOpcodeCount,
	}
	L := lua.NewState(opt)

	// forbid: lua.IoLibName, lua.OsLibName, lua.DebugLibName, lua.ChannelLibName, lua.CoroutineLibName
	worker.openLib(L, lua.LoadLibName, lua.OpenPackage)
	worker.openLib(L, lua.BaseLibName, lua.OpenBase)
	worker.openLib(L, lua.TabLibName, lua.OpenTable)
	worker.openLib(L, lua.StringLibName, lua.OpenString)
	worker.openLib(L, lua.MathLibName, lua.OpenMath)

	return L
}

// openLib loads the built-in libraries. It is equivalent to running OpenLoad,
// then OpenBase, then iterating over the other OpenXXX functions in any order.
func (worker *LuaWorker) openLib(L *lua.LState, libName string, libFunc lua.LGFunction) {
	L.Push(L.NewFunction(libFunc))
	L.Push(lua.LString(libName))
	L.Call(1, 0)
}

// call lua function(L0Init, L0Invoke)
func (worker *LuaWorker) callLuaFunc(L *lua.LState, funcName string, params ...string) (interface{}, error) {
	p := lua.P{
		Fn:      L.GetGlobal(funcName),
		NRet:    1,
		Protect: true,
	}

	//log.Debugf("callLuaFunc, funcName: %+v, Parms: %+v", funcName, params)
	var err error
	l := len(params)
	var lvparams []lua.LValue
	if "L0Invoke" == funcName {
		if l == 0 {
			lvparams = []lua.LValue{lua.LNil, lua.LNil}
		} else if l == 1 {
			lvparams = []lua.LValue{lua.LString(params[0]), lua.LNil}
		} else if l > 1 {
			tb := new(lua.LTable)
			for i := 1; i < l; i++ {
				tb.RawSet(lua.LNumber(i-1), lua.LString(params[i]))
			}
			lvparams = []lua.LValue{lua.LString(params[0]), tb}
		}
	} else {
		if l == 0 {
			lvparams = []lua.LValue{}
		} else if l > 0 {
			tb := new(lua.LTable)
			for i := 0; i < l; i++ {
				tb.RawSet(lua.LNumber(i), lua.LString(params[i]))
			}
			lvparams = []lua.LValue{tb}
		}
	}

	err = L.CallByParam(p, lvparams...)
	if err != nil {
		return false, err
	}

	if _, ok := L.Get(-1).(lua.LBool); ok {
		ret := L.ToBool(-1)
		L.Pop(1) // remove received value
		return ret, nil
	}

	queryResult := L.ToString(-1)
	L.Pop(1) // remove received value
	return queryResult, nil
}