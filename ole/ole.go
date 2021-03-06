package goluaole

import (
	"fmt"
	lua "github.com/zetamatta/go-luadll-wrapper"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

var initialized_required = true

type capsule_t struct {
	Data *ole.IDispatch
}

type method_t struct {
	Name string
	Data *ole.IDispatch
}

const OBJECT_T = "OLE_OBJECT"
const METHOD_T = "OLE_METHOD"

func (this capsule_t) Push(L lua.Lua) int {
	L.PushUserData(&this)
	L.NewMetaTable(OBJECT_T)
	L.PushGoFunction(gc)
	L.SetField(-2, "__gc")
	L.PushGoFunction(index)
	L.SetField(-2, "__index")
	L.PushGoFunction(set)
	L.SetField(-2, "__newindex")
	L.SetMetaTable(-2)
	return 1
}

func gc(L lua.Lua) int {
	defer L.DeleteUserDataAnchor(1)
	p := capsule_t{}
	sync := L.ToUserDataTo(1, &p)
	defer sync()
	if p.Data != nil {
		p.Data.Release()
		p.Data = nil
	}
	return 0
}

func lua2interface(L lua.Lua, index int) (interface{}, error) {
	switch L.GetType(index) {
	default:
		return nil, nil
	case lua.LUA_TSTRING:
		str, str_err := L.ToString(index)
		return str, str_err
	case lua.LUA_TNUMBER:
		num, num_err := L.ToInteger(index)
		return num, num_err
	case lua.LUA_TUSERDATA:
		var val capsule_t
		if _, err := L.TestUDataTo(index, OBJECT_T, &val); err != nil {
			return nil, err
		}
		return val.Data, nil
	case lua.LUA_TBOOLEAN:
		return L.ToBool(index), nil
	}
}

func lua2interfaceS(L lua.Lua, start, end int) ([]interface{}, error) {
	result := make([]interface{}, 0, end-start+1)
	for i := start; i <= end; i++ {
		val, val_err := lua2interface(L, i)
		if val_err != nil {
			return nil, val_err
		}
		result = append(result, val)
	}
	return result, nil
}

// this:_call("METHODNAME",params...)
func call1(L lua.Lua) int {
	var p capsule_t
	if _, err := L.TestUDataTo(1, OBJECT_T, &p); err != nil {
		return L.Push(nil, err)
	}
	name, name_err := L.ToString(2)
	if name_err != nil {
		return L.Push(nil, name_err)
	}
	return call_common(L, p.Data, name)
}

// this:METHODNAME(params...)
func call2(L lua.Lua) int {
	var m method_t
	if _, err := L.TestUDataTo(1, METHOD_T, &m); err != nil {
		return L.Push(nil, fmt.Errorf("%s(%s,call2)", err, METHOD_T))
	}
	if m.Name == "" {
		return L.Push(nil, "OLEOBJECT(): the method is null")
	}
	var p capsule_t
	if _, err := L.TestUDataTo(2, OBJECT_T, &p); err != nil {
		if m.Data == nil {
			return L.Push(nil, fmt.Errorf("%s(%s,call2)", err, OBJECT_T))
		}
		// this code enables `OLEOBJ.PROPERTY.PROPERTY:METHOD()`
		return call_common(L, m.Data, m.Name)
	} else {
		if p.Data == nil {
			return L.Push(nil, "OLEOBJECT(): the receiver is null")
		}
		return call_common(L, p.Data, m.Name)
	}
}

func call_common(L lua.Lua, com1 *ole.IDispatch, name string) int {
	count := L.GetTop()
	params, params_err := lua2interfaceS(L, 3, count)
	if params_err != nil {
		return L.Push(nil, params_err)
	}
	result, result_err := oleutil.CallMethod(com1, name, params...)
	if result_err != nil {
		return L.Push(nil, result_err)
	}
	if result.VT == ole.VT_DISPATCH {
		return capsule_t{result.ToIDispatch()}.Push(L)
	} else {
		return L.Push(result.Value())
	}
}

func set(L lua.Lua) int {
	var p capsule_t
	if _, err := L.TestUDataTo(1, OBJECT_T, &p); err != nil {
		return L.Push(nil, err)
	}
	name, name_err := L.ToString(2)
	if name_err != nil {
		return L.Push(nil, name_err)
	}
	key, key_err := lua2interfaceS(L, 3, L.GetTop())
	if key_err != nil {
		return L.Push(nil, key_err)
	}
	oleutil.PutProperty(p.Data, name, key...)
	return L.Push(true, nil)
}

func get(L lua.Lua) int {
	var p capsule_t
	if _, err := L.TestUDataTo(1, OBJECT_T, &p); err != nil {
		return L.Push(nil, err)
	}
	name, name_err := L.ToString(2)
	if name_err != nil {
		return L.Push(nil, name_err)
	}
	key, key_err := lua2interfaceS(L, 3, L.GetTop())
	if key_err != nil {
		return L.Push(nil, key_err)
	}
	result, result_err := oleutil.GetProperty(p.Data, name, key...)
	if result_err != nil {
		return L.Push(nil, result_err)
	}
	if result.VT == ole.VT_DISPATCH {
		return capsule_t{result.ToIDispatch()}.Push(L)
	} else {
		return L.Push(result.Value())
	}
}

func indexSub(L lua.Lua, thisIndex int, nameIndex int) int {
	name, name_err := L.ToString(nameIndex)
	if name_err != nil {
		return L.Push(nil, name_err)
	}
	switch name {
	case "_call":
		return L.Push(call1, nil)
	case "_set":
		return L.Push(set, nil)
	case "_get":
		return L.Push(get, nil)
	default:
		m := &method_t{Name: name}
		var p capsule_t
		if _, err := L.TestUDataTo(thisIndex, OBJECT_T, &p); err == nil {
			m.Data = p.Data
		}
		L.PushUserData(m)
		L.NewMetaTable(METHOD_T)
		L.PushGoFunction(set)
		L.SetField(-2, "__newindex")
		L.PushGoFunction(call2)
		L.SetField(-2, "__call")
		L.PushGoFunction(get2)
		L.SetField(-2, "__index")
		L.SetGcFunctionForUserData(-2, -1)
		L.SetMetaTable(-2)
		return 1
	}
}

func index(L lua.Lua) int {
	return indexSub(L, 1, 2)
}

// THIS.member.member
func get2(L lua.Lua) int {
	var m method_t
	if _, err := L.TestUDataTo(1, METHOD_T, &m); err != nil {
		println(err.Error())
		return L.Push(nil, err)
	}
	result, err := oleutil.GetProperty(m.Data, m.Name)
	if err != nil {
		println(err.Error())
		return L.Push(nil, err)
	}
	if result.VT == ole.VT_DISPATCH {
		capsule_t{result.ToIDispatch()}.Push(L)
	} else {
		L.Push(result.Value())
	}
	return indexSub(L, 3, 2)
}

func CreateObject(L lua.Lua) int {
	if initialized_required {
		ole.CoInitialize(0)
		initialized_required = false
	}
	name, name_err := L.ToString(1)
	if name_err != nil {
		return L.Push(nil, name_err)
	}
	unknown, unknown_err := oleutil.CreateObject(name)
	if unknown_err != nil {
		return L.Push(nil, unknown_err)
	}
	obj, obj_err := unknown.QueryInterface(ole.IID_IDispatch)
	if obj_err != nil {
		return L.Push(nil, obj_err)
	}
	capsule_t{obj}.Push(L)
	return 1
}
